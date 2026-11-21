// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

func getSettings(t *testing.T, h http.Handler) api.Settings {
	t.Helper()
	var s api.Settings
	if code, ec := call(t, h, http.MethodGet, "/v1/settings", nil, &s); code != http.StatusOK {
		t.Fatalf("GET /v1/settings = %d %s", code, ec)
	}
	return s
}

func putAutoconnect(t *testing.T, h http.Handler, on bool) api.Settings {
	t.Helper()
	var s api.Settings
	if code, ec := call(t, h, http.MethodPut, "/v1/settings", api.SettingsRequest{Autoconnect: &on}, &s); code != http.StatusOK {
		t.Fatalf("PUT /v1/settings = %d %s", code, ec)
	}
	return s
}

// hasLog reports whether the buffer holds a line whose message contains msg.
func hasLog(logs *LogBuffer, msg string) bool {
	for _, l := range logs.Since(0).Lines {
		if strings.Contains(l.Msg, msg) {
			return true
		}
	}
	return false
}

func TestSettingsDefaultOff(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newTestServerWith(t, &fakeEngine{}, dir)
	h := srv.Handler()
	if s := getSettings(t, h); s.Autoconnect {
		t.Fatal("autoconnect on without a settings file")
	}

	// Importing a config doesn't turn it on, or write the file.
	putConfig(t, h, testConfig())
	if s := getSettings(t, h); s.Autoconnect {
		t.Fatal("autoconnect on after importing a config")
	}
	if _, err := os.Stat(filepath.Join(dir, settingsFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("settings file after import: %v", err)
	}
}

func TestPutSettingsPersists(t *testing.T) {
	dir := t.TempDir()
	srv, logs := newTestServerWith(t, &fakeEngine{}, dir)
	h := srv.Handler()

	if s := putAutoconnect(t, h, true); !s.Autoconnect {
		t.Fatalf("PUT response = %+v", s)
	}
	if s := getSettings(t, h); !s.Autoconnect {
		t.Fatal("GET after PUT: autoconnect off")
	}
	if !hasLog(logs, "settings changed") {
		t.Error("no log line for the change")
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, settingsFile))
		if err != nil {
			t.Fatal(err)
		}
		if m := fi.Mode().Perm(); m != 0o600 {
			t.Errorf("settings file mode = %o, want 600", m)
		}
	}

	// A restarted service reads it back.
	srv2, _ := newTestServerWith(t, &fakeEngine{}, dir)
	if s := getSettings(t, srv2.Handler()); !s.Autoconnect {
		t.Fatal("autoconnect lost across a restart")
	}

	putAutoconnect(t, h, false)
	srv3, _ := newTestServerWith(t, &fakeEngine{}, dir)
	if s := getSettings(t, srv3.Handler()); s.Autoconnect {
		t.Fatal("turning autoconnect off didn't persist")
	}
}

func TestPutSettingsRejects(t *testing.T) {
	srv, _ := newTestServerWith(t, &fakeEngine{}, t.TempDir())
	h := srv.Handler()
	for name, body := range map[string]string{
		"bad JSON":      "{",
		"missing field": "{}",
		"unknown field": `{"autoconnect": true, "auto_check": true}`,
		"not a bool":    `{"autoconnect": "yes"}`,
		"null":          `{"autoconnect": null}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/v1/settings", strings.NewReader(body)))
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), string(api.CodeBadRequest)) {
				t.Fatalf("%s = %d %s", body, rec.Code, rec.Body.String())
			}
		})
	}
	if s := getSettings(t, h); s.Autoconnect {
		t.Fatal("a refused request changed the settings")
	}
}

func TestDamagedSettingsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settingsFile), []byte(`{"autoconnect": tr`), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, logs := newTestServerWith(t, &fakeEngine{}, dir)
	if s := getSettings(t, srv.Handler()); s.Autoconnect {
		t.Fatal("autoconnect on with a damaged file")
	}
	if !hasLog(logs, "settings file is damaged") {
		t.Fatal("no warning for a damaged settings file")
	}
}

// newAutoconnectServer returns a server with autoconnect on and, if
// configured, a config imported.
func newAutoconnectServer(t *testing.T, e *fakeEngine, configured bool) (*Server, *LogBuffer) {
	t.Helper()
	srv, logs := newTestServerWith(t, e, t.TempDir())
	if configured {
		putConfig(t, srv.Handler(), testConfig())
	}
	putAutoconnect(t, srv.Handler(), true)
	return srv, logs
}

func TestAutoconnect(t *testing.T) {
	e := &fakeEngine{}
	srv, logs := newAutoconnectServer(t, e, true)
	srv.autoconnect(t.Context())
	if s := status(t, srv.Handler()); s.State != api.StateRunning {
		t.Fatalf("state = %s, want running", s.State)
	}
	if !hasLog(logs, "connecting automatically") {
		t.Fatal("no log line for connecting automatically")
	}
}

func TestAutoconnectSkipped(t *testing.T) {
	t.Run("without a config", func(t *testing.T) {
		e := &fakeEngine{}
		srv, _ := newAutoconnectServer(t, e, false)
		srv.autoconnect(t.Context())
		if s := status(t, srv.Handler()); s.State != api.StateStopped || e.attempts != 0 {
			t.Fatalf("state = %s, attempts = %d", s.State, e.attempts)
		}
	})
	t.Run("setting off", func(t *testing.T) {
		e := &fakeEngine{}
		srv, _ := newTestServerWith(t, e, t.TempDir())
		putConfig(t, srv.Handler(), testConfig())
		srv.autoconnect(t.Context())
		if s := status(t, srv.Handler()); s.State != api.StateStopped || e.attempts != 0 {
			t.Fatalf("state = %s, attempts = %d", s.State, e.attempts)
		}
	})
	t.Run("config rejected", func(t *testing.T) {
		// A cert that expired since the import: no retries, stays stopped
		// as a manual connect does.
		e := &fakeEngine{}
		srv, _ := newAutoconnectServer(t, e, true)
		e.setStartErr(&nebulactl.Error{Reason: nebulactl.ReasonCertExpired, Err: errors.New("certificate is expired")})
		srv.sleep = func(context.Context, time.Duration) bool { t.Fatal("retried a rejected config"); return false }
		srv.autoconnect(t.Context())
		if s := status(t, srv.Handler()); s.State != api.StateStopped || e.attempts != 1 {
			t.Fatalf("state = %s, attempts = %d", s.State, e.attempts)
		}
	})
}

// portInUse is a start failure with a reason, as the real engine reports.
var portInUse = &nebulactl.Error{Reason: nebulactl.ReasonPortInUse, Err: errFakeStart}

func TestAutoconnectFailureGivesUp(t *testing.T) {
	e := &fakeEngine{startErr: portInUse}
	srv, logs := newAutoconnectServer(t, e, true)
	var waits []time.Duration
	srv.sleep = func(_ context.Context, d time.Duration) bool { waits = append(waits, d); return true }

	srv.autoconnect(t.Context())

	if e.attempts != autoconnectRetries+1 {
		t.Fatalf("attempts = %d, want %d", e.attempts, autoconnectRetries+1)
	}
	if len(waits) != autoconnectRetries || waits[0] != autoconnectDelay {
		t.Fatalf("waits = %v", waits)
	}
	s := status(t, srv.Handler())
	if s.State != api.StateError || s.ErrorReason != api.ReasonPortInUse || !strings.Contains(s.Error, "address already in use") {
		t.Fatalf("status = %+v", s)
	}
	if !hasLog(logs, "giving up connecting automatically") {
		t.Fatal("no log line for giving up")
	}
}

func TestAutoconnectRetrySucceeds(t *testing.T) {
	e := &fakeEngine{startErr: portInUse}
	srv, _ := newAutoconnectServer(t, e, true)
	srv.sleep = func(context.Context, time.Duration) bool {
		e.setStartErr(nil) // the network came up
		return true
	}
	srv.autoconnect(t.Context())
	if s := status(t, srv.Handler()); s.State != api.StateRunning || e.attempts != 2 {
		t.Fatalf("state = %s, attempts = %d", s.State, e.attempts)
	}
}

func TestAutoconnectRetryStops(t *testing.T) {
	tests := []struct {
		name string
		// during runs while autoconnect waits to retry.
		during func(t *testing.T, h http.Handler)
	}{
		{"user disconnects", func(t *testing.T, h http.Handler) {
			call(t, h, http.MethodPost, "/v1/disconnect", nil, nil)
		}},
		{"setting turned off", func(t *testing.T, h http.Handler) {
			putAutoconnect(t, h, false)
		}},
		{"config deleted", func(t *testing.T, h http.Handler) {
			call(t, h, http.MethodDelete, "/v1/config", nil, nil)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeEngine{startErr: portInUse}
			srv, _ := newAutoconnectServer(t, e, true)
			srv.sleep = func(context.Context, time.Duration) bool {
				tc.during(t, srv.Handler())
				return true
			}
			srv.autoconnect(t.Context())
			if e.attempts != 1 {
				t.Fatalf("attempts = %d, want 1", e.attempts)
			}
		})
	}

	t.Run("service stops", func(t *testing.T) {
		e := &fakeEngine{startErr: portInUse}
		srv, _ := newAutoconnectServer(t, e, true)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		srv.autoconnectDelay = time.Hour // only ctx can end the wait
		srv.autoconnect(ctx)
		if e.attempts != 1 {
			t.Fatalf("attempts = %d, want 1", e.attempts)
		}
	})
}
