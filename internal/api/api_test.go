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
	want := VersionResponse{ServiceVersion: "0.1.0", NebulaVersion: "1.12.0", APIVersion: APIVersion, InstanceID: "abc"}
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

func TestClientDiagnostics(t *testing.T) {
	var queries []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/diagnostics" {
			http.NotFound(w, r)
			return
		}
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"instance_id":"abc","status":{"state":"stopped"},"config_summary":null,` +
			`"log":["{\"msg\":\"service started\"}"],"log_truncated":true}`))
	}))
	for _, lines := range []int{0, 50} {
		d, err := c.Diagnostics(context.Background(), lines)
		if err != nil {
			t.Fatalf("Diagnostics(%d): %v", lines, err)
		}
		if d.InstanceID != "abc" || d.Status.State != StateStopped || d.ConfigSummary != nil ||
			len(d.Log) != 1 || d.Log[0] != `{"msg":"service started"}` || !d.LogTruncated {
			t.Fatalf("Diagnostics(%d) = %+v", lines, d)
		}
	}
	if len(queries) != 2 || queries[0] != "" || queries[1] != "lines=50" {
		t.Fatalf("queries = %q", queries)
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

func TestClientPeers(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/peers" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"peers":[{"vpn_ips":["192.168.100.10"],"name":"alice","groups":["users"],` +
			`"state":"established","current_remote":"203.0.113.10:4242","remote_addrs":["203.0.113.10:4242"],` +
			`"relayed":false,"cert_not_after":"2030-01-01T00:00:00Z","last_ping_rtt_ms":null}]}`))
	}))
	r, err := c.Peers(context.Background())
	if err != nil {
		t.Fatalf("Peers: %v", err)
	}
	if len(r.Peers) != 1 {
		t.Fatalf("peers = %+v", r.Peers)
	}
	p := r.Peers[0]
	if p.Name != "alice" || p.State != PeerEstablished || p.VpnIPs[0] != "192.168.100.10" ||
		p.CertNotAfter == nil || p.LastPingRTTMs != nil {
		t.Fatalf("peer = %+v", p)
	}
}

func TestClientClosePeer(t *testing.T) {
	var got string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		got = r.URL.Path
		if r.URL.Path == "/v1/peers/192.168.100.99/close" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"no tunnel to that address"}}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if err := c.ClosePeer(context.Background(), "192.168.100.10"); err != nil {
		t.Fatalf("ClosePeer: %v", err)
	}
	if got != "/v1/peers/192.168.100.10/close" {
		t.Fatalf("path = %q", got)
	}
	err := c.ClosePeer(context.Background(), "192.168.100.99")
	if ErrorCodeOf(err) != CodeNotFound {
		t.Fatalf("ClosePeer without tunnel: %v", err)
	}
}

func TestClientPing(t *testing.T) {
	var got PingRequest
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/ping" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		if got.VpnIP == "192.168.100.11" {
			_, _ = w.Write([]byte(`{"vpn_ip":"192.168.100.11","sent":3,"received":0}`))
			return
		}
		_, _ = w.Write([]byte(`{"vpn_ip":"192.168.100.10","sent":3,"received":3,"rtt_min_ms":21,"rtt_avg_ms":23.4,"rtt_max_ms":27.9}`))
	}))
	r, err := c.Ping(context.Background(), PingRequest{VpnIP: "192.168.100.10"})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if got.Count != 0 || got.TimeoutMs != 0 {
		t.Fatalf("defaults sent explicitly: %+v", got)
	}
	if r.Received != 3 || r.RTTAvgMs == nil || *r.RTTAvgMs != 23.4 {
		t.Fatalf("ping = %+v", r)
	}
	r, err = c.Ping(context.Background(), PingRequest{VpnIP: "192.168.100.11", Count: 1})
	if err != nil || r.Received != 0 || r.RTTAvgMs != nil || got.Count != 1 {
		t.Fatalf("unanswered ping = %+v, %v (sent %+v)", r, err, got)
	}
}
