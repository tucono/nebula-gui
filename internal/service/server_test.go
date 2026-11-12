// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tucono/nebula-gui/internal/api"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, _ := newTestServerWith(t, &fakeEngine{}, t.TempDir())
	return s
}

func newTestServerWith(t *testing.T, e *fakeEngine, dir string) (*Server, *LogBuffer) {
	t.Helper()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	logs := NewLogBuffer(100)
	s, err := New(Options{
		Version: "1.2.3-test",
		Logger:  slog.New(logs.Handler(slog.LevelInfo)),
		Engine:  e,
		Store:   store,
		Logs:    logs,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, logs
}

func get(t *testing.T, h http.Handler, path string, out any) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	resp := rec.Result()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
	}
	return resp
}

func TestVersion(t *testing.T) {
	var v api.VersionResponse
	resp := get(t, newTestServer(t).Handler(), "/v1/version", &v)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if v.ServiceVersion != "1.2.3-test" || v.APIVersion != api.APIVersion || v.NebulaVersion != fakeNebulaVersion {
		t.Fatalf("unexpected version: %+v", v)
	}
}

func TestStatusStoppedWithoutConfig(t *testing.T) {
	var s api.StatusResponse
	get(t, newTestServer(t).Handler(), "/v1/status", &s)
	if s.State != api.StateStopped || s.Configured || s.OverlayIPs == nil {
		t.Fatalf("unexpected status: %+v", s)
	}
}

func TestWrongMethod(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer(t).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/version", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
