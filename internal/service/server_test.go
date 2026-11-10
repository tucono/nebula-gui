// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tucono/nebula-gui/internal/api"
)

func newTestServer() *Server {
	return New(Options{Version: "1.2.3-test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
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
	resp := get(t, newTestServer().Handler(), "/v1/version", &v)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if v.ServiceVersion != "1.2.3-test" || v.APIVersion != api.APIVersion {
		t.Fatalf("unexpected version: %+v", v)
	}
}

func TestStatusStoppedWithoutConfig(t *testing.T) {
	var s api.StatusResponse
	get(t, newTestServer().Handler(), "/v1/status", &s)
	if s.State != api.StateStopped || s.Configured || s.OverlayIPs == nil {
		t.Fatalf("unexpected status: %+v", s)
	}
}

func TestWrongMethod(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/version", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
