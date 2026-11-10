// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusIgnoresUnknownFields(t *testing.T) {
	in := `{"state":"running","overlay_ips":["192.168.100.20/24"],"configured":true,"added_later":{"x":1}}`
	var s StatusResponse
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.State != StateRunning || !s.Configured || len(s.OverlayIPs) != 1 {
		t.Fatalf("unexpected status: %+v", s)
	}
}

func TestVersionRoundTrip(t *testing.T) {
	want := VersionResponse{ServiceVersion: "0.1.0", NebulaVersion: "1.12.0", APIVersion: APIVersion}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got VersionResponse
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// testClient returns a client whose connections go to srv over TCP instead
// of the IPC transport.
func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	return newClient("test", func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	})
}

func TestClientVersion(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"service_version":"0.1.0","nebula_version":"1.12.0","api_version":1}`))
	}))
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.ServiceVersion != "0.1.0" || v.APIVersion != 1 {
		t.Fatalf("unexpected version: %+v", v)
	}
}

func TestClientDecodesServiceError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"not_configured","message":"no config has been imported"}}`))
	}))
	_, err := c.Status(context.Background())
	if got := ErrorCodeOf(err); got != CodeNotConfigured {
		t.Fatalf("error code = %q, want %q (err: %v)", got, CodeNotConfigured, err)
	}
	var e *Error
	if !errors.As(err, &e) || e.HTTPStatus != http.StatusConflict {
		t.Fatalf("expected *Error with status 409, got %v", err)
	}
}

func TestClientNonJSONError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	_, err := c.Version(context.Background())
	if got := ErrorCodeOf(err); got != CodeInternal {
		t.Fatalf("error code = %q, want %q (err: %v)", got, CodeInternal, err)
	}
}
