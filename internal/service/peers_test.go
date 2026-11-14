// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

func peers(t *testing.T, h http.Handler) []api.Peer {
	t.Helper()
	var r api.PeersResponse
	if code, ec := call(t, h, http.MethodGet, "/v1/peers", nil, &r); code != http.StatusOK {
		t.Fatalf("GET /v1/peers = %d %s", code, ec)
	}
	if r.Peers == nil {
		t.Fatal("peers is null, want []")
	}
	return r.Peers
}

// runningServer returns a server with Nebula (fake) running.
func runningServer(t *testing.T) (http.Handler, *fakeEngine) {
	t.Helper()
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	h := srv.Handler()
	putConfig(t, h, testConfig())
	if code, ec := call(t, h, http.MethodPost, "/v1/connect", nil, nil); code != http.StatusOK {
		t.Fatalf("connect = %d %s", code, ec)
	}
	return h, e
}

func host(ip, name string) nebulactl.HostInfo {
	return nebulactl.HostInfo{VpnAddrs: []netip.Addr{netip.MustParseAddr(ip)}, Name: name}
}

func TestPeersWhileStopped(t *testing.T) {
	h := newTestServer(t).Handler()
	if p := peers(t, h); len(p) != 0 {
		t.Fatalf("peers while stopped = %+v", p)
	}
	putConfig(t, h, testConfig())
	if p := peers(t, h); len(p) != 0 {
		t.Fatalf("peers with config, stopped = %+v", p)
	}
}

func TestPeersListing(t *testing.T) {
	h, e := runningServer(t)
	if p := peers(t, h); len(p) != 0 {
		t.Fatalf("peers with an empty hostmap = %+v", p)
	}

	notAfter := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	alice := nebulactl.HostInfo{
		VpnAddrs:      []netip.Addr{netip.MustParseAddr("192.168.100.10")},
		Name:          "alice",
		Groups:        []string{"users"},
		CertNotAfter:  notAfter,
		CurrentRemote: netip.MustParseAddrPort("203.0.113.10:4242"),
		RemoteAddrs: []netip.AddrPort{
			netip.MustParseAddrPort("203.0.113.10:4242"),
			netip.MustParseAddrPort("10.0.0.10:4242"),
		},
	}
	lighthouse := host("192.168.100.1", "lighthouse")
	relayed := host("192.168.100.12", "")
	relayed.Relayed = true
	bobPending := host("192.168.100.11", "")
	// A host mid-rehandshake is in both maps; it's listed as established.
	alicePending := host("192.168.100.10", "")
	e.instance().setHosts(
		[]nebulactl.HostInfo{relayed, alice, lighthouse},
		[]nebulactl.HostInfo{alicePending, bobPending},
	)

	p := peers(t, h)
	var got []string
	for _, x := range p {
		got = append(got, x.VpnIPs[0]+" "+string(x.State))
	}
	want := []string{
		"192.168.100.1 established",
		"192.168.100.10 established",
		"192.168.100.12 established",
		"192.168.100.11 pending",
	}
	if len(got) != len(want) {
		t.Fatalf("peers = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("peers = %q, want %q", got, want)
		}
	}

	a := p[1]
	if a.Name != "alice" || len(a.Groups) != 1 || a.Groups[0] != "users" ||
		a.CurrentRemote != "203.0.113.10:4242" || len(a.RemoteAddrs) != 2 || a.Relayed ||
		a.CertNotAfter == nil || !a.CertNotAfter.Equal(notAfter) || a.LastPingRTTMs != nil {
		t.Fatalf("alice = %+v", a)
	}
	r := p[2]
	if !r.Relayed || r.CurrentRemote != "" || r.CertNotAfter != nil || r.Groups == nil || r.RemoteAddrs == nil {
		t.Fatalf("relayed = %+v", r)
	}

	// Disconnecting empties the list.
	call(t, h, http.MethodPost, "/v1/disconnect", nil, nil)
	if p := peers(t, h); len(p) != 0 {
		t.Fatalf("peers after disconnect = %+v", p)
	}
}

func TestClosePeer(t *testing.T) {
	stopped := newTestServer(t).Handler()
	if code, ec := call(t, stopped, http.MethodPost, "/v1/peers/192.168.100.10/close", nil, nil); code != http.StatusConflict || ec != api.CodeNotRunning {
		t.Fatalf("close while stopped = %d %s", code, ec)
	}

	h, e := runningServer(t)
	inst := e.instance()
	inst.setHosts([]nebulactl.HostInfo{host("192.168.100.10", "alice")}, []nebulactl.HostInfo{host("192.168.100.11", "")})

	for _, tc := range []struct {
		path string
		code int
		ec   api.ErrorCode
	}{
		{"/v1/peers/not-an-ip/close", http.StatusBadRequest, api.CodeBadRequest},
		{"/v1/peers/192.168.100.99/close", http.StatusNotFound, api.CodeNotFound},
		// Pending handshakes can't be closed, as in Nebula.
		{"/v1/peers/192.168.100.11/close", http.StatusNotFound, api.CodeNotFound},
		{"/v1/peers/192.168.100.10/close", http.StatusNoContent, ""},
		{"/v1/peers/192.168.100.10/close", http.StatusNotFound, api.CodeNotFound},
	} {
		if code, ec := call(t, h, http.MethodPost, tc.path, nil, nil); code != tc.code || ec != tc.ec {
			t.Errorf("POST %s = %d %s, want %d %s", tc.path, code, ec, tc.code, tc.ec)
		}
	}
	if len(inst.closed) != 1 || inst.closed[0] != netip.MustParseAddr("192.168.100.10") {
		t.Fatalf("closed = %v", inst.closed)
	}
	for _, x := range peers(t, h) {
		if x.VpnIPs[0] == "192.168.100.10" {
			t.Fatalf("closed peer still listed: %+v", x)
		}
	}
}
