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
)

// call sends a request and decodes the response into out (if non-nil). It
// returns the status code and, for error responses, the error code.
func call(t *testing.T, h http.Handler, method, path string, in, out any) (int, api.ErrorCode) {
	t.Helper()
	var body *bytes.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(b)
	} else {
		body = bytes.NewReader(nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, body))
	if rec.Code >= 400 {
		var eb api.ErrorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &eb); err != nil || eb.Error == nil {
			t.Fatalf("%s %s: bad error body %q", method, path, rec.Body.String())
		}
		return rec.Code, eb.Error.Code
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return rec.Code, ""
}

func status(t *testing.T, h http.Handler) api.StatusResponse {
	t.Helper()
	var s api.StatusResponse
	call(t, h, http.MethodGet, "/v1/status", nil, &s)
	return s
}

func putConfig(t *testing.T, h http.Handler, yaml string) api.ConfigPutResponse {
	t.Helper()
	var r api.ConfigPutResponse
	if code, ec := call(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{YAML: yaml}, &r); code != http.StatusOK {
		t.Fatalf("PUT /v1/config = %d %s", code, ec)
	}
	return r
}

func TestConnectLifecycle(t *testing.T) {
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()

	if code, ec := call(t, h, http.MethodPost, "/v1/connect", nil, nil); code != http.StatusConflict || ec != api.CodeNotConfigured {
		t.Fatalf("connect without config = %d %s", code, ec)
	}

	if r := putConfig(t, h, testConfig()); r.RestartRequired {
		t.Fatal("restart_required while stopped")
	}
	s := status(t, h)
	if !s.Configured || s.State != api.StateStopped || s.Cert == nil || s.Cert.Name != "dev" ||
		len(s.OverlayIPs) != 1 || s.ListenPort != 4242 || s.StartedAt != nil {
		t.Fatalf("status after import = %+v", s)
	}

	var cs api.StatusResponse
	if code, ec := call(t, h, http.MethodPost, "/v1/connect", nil, &cs); code != http.StatusOK {
		t.Fatalf("connect = %d %s", code, ec)
	}
	if cs.State != api.StateRunning || cs.StartedAt == nil {
		t.Fatalf("status after connect = %+v", cs)
	}
	if code, ec := call(t, h, http.MethodPost, "/v1/connect", nil, nil); ec != api.CodeAlreadyRunning {
		t.Fatalf("second connect = %d %s", code, ec)
	}

	// A new config while running is stored but not applied.
	if r := putConfig(t, h, testConfig()+"# alice\n"); !r.RestartRequired {
		t.Fatal("restart_required not set while running")
	}
	if s := status(t, h); s.Cert.Name != "dev" {
		t.Fatalf("status shows the pending config's cert while running: %+v", s.Cert)
	}

	inst := e.instance()
	if code, _ := call(t, h, http.MethodPost, "/v1/disconnect", nil, &cs); code != http.StatusOK || cs.State != api.StateStopped {
		t.Fatalf("disconnect = %d %+v", code, cs)
	}
	if !inst.stopped {
		t.Fatal("instance not stopped")
	}
	if s := status(t, h); s.Cert.Name != "alice" {
		t.Fatalf("status after disconnect should show the stored config: %+v", s.Cert)
	}
	if _, ec := call(t, h, http.MethodPost, "/v1/disconnect", nil, nil); ec != api.CodeNotRunning {
		t.Fatalf("second disconnect = %s", ec)
	}
}

func TestStartFailure(t *testing.T) {
	e := &fakeEngine{startErr: errFakeStart}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig())

	if code, ec := call(t, h, http.MethodPost, "/v1/connect", nil, nil); code != http.StatusInternalServerError || ec != api.CodeInternal {
		t.Fatalf("connect = %d %s", code, ec)
	}
	s := status(t, h)
	if s.State != api.StateError || !strings.Contains(s.Error, "address already in use") {
		t.Fatalf("status = %+v", s)
	}

	// Disconnect clears the error.
	if code, _ := call(t, h, http.MethodPost, "/v1/disconnect", nil, nil); code != http.StatusOK {
		t.Fatalf("disconnect from error = %d", code)
	}
	if s := status(t, h); s.State != api.StateStopped || s.Error != "" {
		t.Fatalf("status = %+v", s)
	}

	// Connect from error retries.
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	e.mu.Lock()
	e.startErr = nil
	e.mu.Unlock()
	if code, _ := call(t, h, http.MethodPost, "/v1/connect", nil, nil); code != http.StatusOK {
		t.Fatalf("connect retry = %d", code)
	}
}

func TestNebulaExitsOnItsOwn(t *testing.T) {
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
			if !strings.Contains(s.Error, "bad file descriptor") {
				t.Fatalf("error = %q", s.Error)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %s, want error", s.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code, _ := call(t, h, http.MethodPost, "/v1/connect", nil, nil); code != http.StatusOK {
		t.Fatalf("reconnect after crash = %d", code)
	}
}

func TestPutConfigRejects(t *testing.T) {
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/v1/config", strings.NewReader("{")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON = %d", rec.Code)
	}
	if _, ec := call(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{}, nil); ec != api.CodeBadRequest {
		t.Fatalf("empty yaml = %s", ec)
	}

	big := api.ConfigRequest{YAML: strings.Repeat("#", maxBodyBytes)}
	if code, ec := call(t, h, http.MethodPut, "/v1/config", big, nil); code != http.StatusBadRequest || ec != api.CodeBadRequest {
		t.Fatalf("oversized body = %d %s", code, ec)
	}

	path := strings.Replace(testConfig(), "  key: |\n"+indentPEM(testPEM("TEST KEY")), "  key: /etc/shadow\n", 1)
	if code, ec := call(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{YAML: path}, nil); code != http.StatusUnprocessableEntity || ec != api.CodeInvalidConfig {
		t.Fatalf("path pki.key = %d %s", code, ec)
	}

	e.checkErr = errors.New("no networks in certificate")
	if _, ec := call(t, h, http.MethodPut, "/v1/config", api.ConfigRequest{YAML: testConfig()}, nil); ec != api.CodeInvalidConfig {
		t.Fatalf("dry run failure = %s", ec)
	}
	if s := status(t, h); s.Configured {
		t.Fatal("a rejected config was stored")
	}
}

func TestGetAndDeleteConfig(t *testing.T) {
	e := &fakeEngine{}
	dir := t.TempDir()
	srv, _ := newTestServerWith(t, e, dir)
	h := srv.Handler()

	if _, ec := call(t, h, http.MethodGet, "/v1/config", nil, nil); ec != api.CodeNotConfigured {
		t.Fatalf("GET without config = %s", ec)
	}
	putConfig(t, h, testConfig())

	var cr api.ConfigResponse
	call(t, h, http.MethodGet, "/v1/config", nil, &cr)
	if strings.Contains(cr.YAML, "TEST KEY") || !strings.Contains(cr.YAML, "TEST CERT") {
		t.Fatalf("GET /v1/config:\n%s", cr.YAML)
	}

	// The config survives a service restart.
	srv2, _ := newTestServerWith(t, e, dir)
	if s := status(t, srv2.Handler()); !s.Configured || s.Cert == nil {
		t.Fatalf("status after restart = %+v", s)
	}

	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	inst := e.instance()
	var s api.StatusResponse
	if code, _ := call(t, h, http.MethodDelete, "/v1/config", nil, &s); code != http.StatusOK {
		t.Fatalf("DELETE = %d", code)
	}
	if !inst.stopped || s.State != api.StateStopped || s.Configured || s.Cert != nil {
		t.Fatalf("after DELETE: stopped=%v status=%+v", inst.stopped, s)
	}
	if _, ec := call(t, h, http.MethodGet, "/v1/config", nil, nil); ec != api.CodeNotConfigured {
		t.Fatalf("GET after DELETE = %s", ec)
	}
}

func TestLogsEndpoint(t *testing.T) {
	srv, _ := newTestServerWith(t, &fakeEngine{}, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig()) // logs "config imported"

	var r api.LogsResponse
	call(t, h, http.MethodGet, "/v1/logs", nil, &r)
	if len(r.Lines) == 0 || r.Next == 0 {
		t.Fatalf("logs = %+v", r)
	}
	for _, l := range r.Lines {
		if strings.Contains(l.Msg, "TEST KEY") {
			t.Fatal("config contents in the log")
		}
	}
	var r2 api.LogsResponse
	call(t, h, http.MethodGet, "/v1/logs?since="+jsonNum(r.Next), nil, &r2)
	if len(r2.Lines) != 0 || r2.Next != r.Next {
		t.Fatalf("logs since next = %+v", r2)
	}
	if _, ec := call(t, h, http.MethodGet, "/v1/logs?since=-1", nil, nil); ec != api.CodeBadRequest {
		t.Fatalf("bad since = %s", ec)
	}
}

func TestCloseStopsNebula(t *testing.T) {
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig())
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	srv.Close()
	if !e.instance().stopped {
		t.Fatal("Close did not stop nebula")
	}
}

func jsonNum(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
