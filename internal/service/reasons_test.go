// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

// callErr sends a request that must fail and returns the decoded error.
func callErr(t *testing.T, h http.Handler, method, path string, in any) *api.Error {
	t.Helper()
	var body bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&body).Encode(in); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, &body))
	var eb api.ErrorBody
	if rec.Code < 400 || json.Unmarshal(rec.Body.Bytes(), &eb) != nil || eb.Error == nil {
		t.Fatalf("%s %s = %d %q, want an error", method, path, rec.Code, rec.Body.String())
	}
	eb.Error.HTTPStatus = rec.Code
	return eb.Error
}

func TestValidateConfigReasons(t *testing.T) {
	good := testConfig()
	tests := []struct {
		name, yaml string
		want       api.Reason
	}{
		{"not yaml", "pki: [", api.ReasonYAMLSyntax},
		{"not a mapping", "- a\n- b\n", api.ReasonYAMLSyntax},
		{"no pki", "listen:\n  port: 4242\n", api.ReasonPKIMissing},
		{"missing key", strings.Replace(good, "  key: |", "  other: |", 1), api.ReasonPKIMissing},
		{"sshd enabled", good + "sshd:\n  enabled: true\n", api.ReasonSSHDEnabled},
		// The GUI inlines files before sending, so a path here is a bug
		// in the client, not something to explain to the user.
		{"key is a path", strings.Replace(good, "  key: |\n"+indentPEM(testPEM("TEST KEY")), "  key: /etc/shadow\n", 1), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ic *errInvalidConfig
			if err := validateConfig(tc.yaml); !errors.As(err, &ic) || ic.reason != tc.want {
				t.Fatalf("err = %v, reason %q, want %q", err, reasonField(ic), tc.want)
			}
		})
	}
}

func reasonField(ic *errInvalidConfig) api.Reason {
	if ic == nil {
		return ""
	}
	return ic.reason
}

func TestPutConfigReason(t *testing.T) {
	e := &fakeEngine{checkErr: &nebulactl.Error{Reason: nebulactl.ReasonKeyMismatch, Err: errors.New("key and cert are not a pair")}}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()

	ae := callErr(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{YAML: testConfig()})
	if ae.Code != api.CodeInvalidConfig || ae.Reason != api.ReasonKeyMismatch || !strings.Contains(ae.Message, "not a pair") {
		t.Fatalf("error = %+v", ae)
	}

	ae = callErr(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{YAML: testConfig() + "sshd:\n  enabled: true\n"})
	if ae.Reason != api.ReasonSSHDEnabled {
		t.Fatalf("error = %+v", ae)
	}

	// An unclassified dry-run failure has no reason.
	e.checkErr = errors.New("no networks in certificate")
	if ae := callErr(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{YAML: testConfig()}); ae.Reason != "" {
		t.Fatalf("error = %+v", ae)
	}
}

// A cert that was valid at import but has expired since then shows as
// expired in the status, and Connect says why instead of "internal".
func TestCertExpiresAfterImport(t *testing.T) {
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig())

	if s := status(t, h); s.Cert == nil || s.Cert.Expired || s.Cert.CAExpired ||
		s.Cert.CANotAfter == nil || !s.Cert.CANotAfter.Equal(fakeCANotAfter) {
		t.Fatalf("cert before expiry = %+v", s.Cert)
	}

	srv.mgr.now = func() time.Time { return fakeCertNotAfter.Add(time.Hour) }
	if s := status(t, h); !s.Cert.Expired || s.Cert.CAExpired {
		t.Fatalf("cert after host expiry = %+v", s.Cert)
	}
	srv.mgr.now = func() time.Time { return fakeCANotAfter.Add(time.Hour) }
	if s := status(t, h); !s.Cert.Expired || !s.Cert.CAExpired {
		t.Fatalf("cert after CA expiry = %+v", s.Cert)
	}

	e.setStartErr(&nebulactl.Error{Reason: nebulactl.ReasonCertExpired, Err: errors.New("nebula certificate for this host expired")})
	ae := callErr(t, h, http.MethodPost, "/v1/connect", nil)
	if ae.Code != api.CodeInvalidConfig || ae.Reason != api.ReasonCertExpired || ae.HTTPStatus != http.StatusUnprocessableEntity {
		t.Fatalf("connect = %+v", ae)
	}
	// Nebula never ran, so there's no error state to dismiss.
	if s := status(t, h); s.State != api.StateStopped || s.Error != "" || s.ErrorReason != "" {
		t.Fatalf("status after refused connect = %+v", s)
	}
}

// After a service restart, a stored config whose cert has expired still
// describes the cert, so the GUI can show the expiry before Connect.
func TestExpiredConfigAfterRestart(t *testing.T) {
	e := &fakeEngine{}
	dir := t.TempDir()
	srv, _ := newTestServerWith(t, e, dir)
	putConfig(t, srv.Handler(), testConfig())

	e.checkErr = &nebulactl.Error{Reason: nebulactl.ReasonCertExpired, Err: errors.New("expired")}
	srv2, _ := newTestServerWith(t, e, dir)
	srv2.mgr.now = func() time.Time { return fakeCertNotAfter.Add(time.Hour) }
	if s := status(t, srv2.Handler()); !s.Configured || s.Cert == nil || !s.Cert.Expired {
		t.Fatalf("status = %+v", s)
	}
}

func TestStartFailureReason(t *testing.T) {
	e := &fakeEngine{startErr: &nebulactl.Error{Reason: nebulactl.ReasonTunUnavailable, Err: errors.New("Failed to get a tun/tap device: operation not permitted")}}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig())

	ae := callErr(t, h, http.MethodPost, "/v1/connect", nil)
	if ae.Code != api.CodeInternal || ae.Reason != api.ReasonTunUnavailable || !strings.Contains(ae.Message, "tun/tap") {
		t.Fatalf("connect = %+v", ae)
	}
	if s := status(t, h); s.State != api.StateError || s.ErrorReason != api.ReasonTunUnavailable {
		t.Fatalf("status = %+v", s)
	}
	call(t, h, http.MethodPost, "/v1/disconnect", nil, nil)
	if s := status(t, h); s.State != api.StateStopped || s.ErrorReason != "" {
		t.Fatalf("status after dismiss = %+v", s)
	}

	// Without a reason, the status has none either.
	e.setStartErr(errFakeStart)
	_ = callErr(t, h, http.MethodPost, "/v1/connect", nil)
	if s := status(t, h); s.State != api.StateError || s.ErrorReason != "" {
		t.Fatalf("status = %+v", s)
	}
}

func TestNebulaStoppedReason(t *testing.T) {
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig())
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)

	e.instance().crash(errors.New("tun read: bad file descriptor"))
	deadline := time.Now().Add(5 * time.Second)
	for {
		s := status(t, h)
		if s.State == api.StateError {
			if s.ErrorReason != api.ReasonNebulaStopped {
				t.Fatalf("status = %+v", s)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %s, want error", s.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	if s := status(t, h); s.State != api.StateRunning || s.ErrorReason != "" {
		t.Fatalf("status after reconnect = %+v", s)
	}
}

// nebulactl's reasons pass through to the API unchanged, so both lists
// must use the same strings.
func TestReasonsMatchAPI(t *testing.T) {
	pairs := map[nebulactl.Reason]api.Reason{
		nebulactl.ReasonCertExpired:    api.ReasonCertExpired,
		nebulactl.ReasonCAExpired:      api.ReasonCAExpired,
		nebulactl.ReasonKeyMismatch:    api.ReasonKeyMismatch,
		nebulactl.ReasonWrongCA:        api.ReasonWrongCA,
		nebulactl.ReasonKeyEncrypted:   api.ReasonKeyEncrypted,
		nebulactl.ReasonTunUnavailable: api.ReasonTunUnavailable,
		nebulactl.ReasonPortInUse:      api.ReasonPortInUse,
	}
	for n, a := range pairs {
		if string(n) != string(a) {
			t.Errorf("nebulactl %q != api %q", n, a)
		}
	}
}
