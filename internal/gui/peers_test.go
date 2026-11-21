// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/tucono/nebula-gui/internal/api"
)

var labOverlay = []netip.Prefix{netip.MustParsePrefix("192.168.100.20/24")}

func apiPeer(state api.PeerState, name string, ips ...string) api.Peer {
	return api.Peer{VpnIPs: ips, Name: name, State: state, Groups: []string{}, RemoteAddrs: []string{}}
}

// rowView is the part of a peerRow the table tests compare.
type rowView struct {
	IP       string
	Title    string
	Subtitle string
	Status   peerStatus
	Actions  peerActions
}

func viewRows(rows []peerRow) []rowView {
	out := make([]rowView, 0, len(rows))
	for _, r := range rows {
		out = append(out, rowView{r.IP.String(), r.Title, r.Subtitle, r.Status, r.Actions})
	}
	return out
}

func TestBuildPeerRows(t *testing.T) {
	rtt := 23.4
	relayed := apiPeer(api.PeerEstablished, "bob", "192.168.100.11")
	relayed.Relayed = true
	relayed.LastPingRTTMs = &rtt

	running := peerActions{Ping: true, Close: true}
	runningSaved := peerActions{Ping: true, Remove: true, Close: true}

	tests := []struct {
		name     string
		peers    []api.Peer
		saved    []savedPeer
		overlays []netip.Prefix
		running  bool
		want     []rowView
	}{
		{
			name:    "nothing",
			running: true,
			want:    []rowView{},
		},
		{
			name:    "cert name, saved name, IP only",
			running: true,
			peers: []api.Peer{
				apiPeer(api.PeerEstablished, "alice", "192.168.100.10"),
				apiPeer(api.PeerEstablished, "bob", "192.168.100.11"),
				apiPeer(api.PeerEstablished, "", "192.168.100.12"),
			},
			saved:    []savedPeer{{"192.168.100.11", "Bob's PC"}},
			overlays: labOverlay,
			want: []rowView{
				{"192.168.100.12", "192.168.100.12", "", peerConnected, running},
				{"192.168.100.10", "alice", "192.168.100.10", peerConnected, running},
				{"192.168.100.11", "Bob's PC", "192.168.100.11 · bob", peerConnected, runningSaved},
			},
		},
		{
			name:     "saved name equal to the cert name isn't repeated",
			running:  true,
			peers:    []api.Peer{apiPeer(api.PeerEstablished, "alice", "192.168.100.10")},
			saved:    []savedPeer{{"192.168.100.10", "alice"}},
			overlays: labOverlay,
			want:     []rowView{{"192.168.100.10", "alice", "192.168.100.10", peerConnected, runningSaved}},
		},
		{
			name:    "connected, then connecting, then not connected; by name within each",
			running: true,
			peers: []api.Peer{
				apiPeer(api.PeerPending, "lighthouse", "192.168.100.1"),
				apiPeer(api.PeerEstablished, "bob", "192.168.100.11"),
				apiPeer(api.PeerEstablished, "Alice", "192.168.100.10"),
			},
			saved:    []savedPeer{{"192.168.100.30", "zed"}, {"192.168.100.31", "Carol"}},
			overlays: labOverlay,
			want: []rowView{
				{"192.168.100.10", "Alice", "192.168.100.10", peerConnected, running},
				{"192.168.100.11", "bob", "192.168.100.11", peerConnected, running},
				{"192.168.100.1", "lighthouse", "192.168.100.1", peerConnecting, peerActions{Ping: true}},
				{"192.168.100.31", "Carol", "192.168.100.31", peerNotConnected, peerActions{Ping: true, Remove: true}},
				{"192.168.100.30", "zed", "192.168.100.30", peerNotConnected, peerActions{Ping: true, Remove: true}},
			},
		},
		{
			name:     "connecting saved peer can't be closed",
			running:  true,
			peers:    []api.Peer{apiPeer(api.PeerPending, "", "192.168.100.10")},
			saved:    []savedPeer{{"192.168.100.10", "Alice's laptop"}},
			overlays: labOverlay,
			want:     []rowView{{"192.168.100.10", "Alice's laptop", "192.168.100.10", peerConnecting, peerActions{Ping: true, Remove: true}}},
		},
		{
			name:     "matched on a second overlay IP",
			running:  true,
			peers:    []api.Peer{apiPeer(api.PeerEstablished, "alice", "192.168.100.10", "fd00::10")},
			saved:    []savedPeer{{"fd00::10", "Alice"}},
			overlays: append([]netip.Prefix{netip.MustParsePrefix("fd00::20/64")}, labOverlay...),
			want:     []rowView{{"192.168.100.10", "Alice", "192.168.100.10 · alice", peerConnected, runningSaved}},
		},
		{
			name:     "saved peers outside the network are hidden",
			running:  true,
			saved:    []savedPeer{{"192.168.100.10", "Alice"}, {"10.1.0.5", "Old network"}},
			overlays: labOverlay,
			want:     []rowView{{"192.168.100.10", "Alice", "192.168.100.10", peerNotConnected, peerActions{Ping: true, Remove: true}}},
		},
		{
			name:    "no network known: every saved peer shows",
			saved:   []savedPeer{{"192.168.100.10", "Alice"}, {"10.1.0.5", "Old network"}},
			running: false,
			want: []rowView{
				{"192.168.100.10", "Alice", "192.168.100.10", peerNotConnected, peerActions{Remove: true}},
				{"10.1.0.5", "Old network", "10.1.0.5", peerNotConnected, peerActions{Remove: true}},
			},
		},
		{
			name:     "not running: Nebula's peers ignored, no ping",
			running:  false,
			peers:    []api.Peer{apiPeer(api.PeerEstablished, "bob", "192.168.100.11")},
			saved:    []savedPeer{{"192.168.100.10", "Alice"}},
			overlays: labOverlay,
			want:     []rowView{{"192.168.100.10", "Alice", "192.168.100.10", peerNotConnected, peerActions{Remove: true}}},
		},
		{
			name:    "peer without a usable IP is skipped",
			running: true,
			peers:   []api.Peer{apiPeer(api.PeerEstablished, "ghost"), apiPeer(api.PeerEstablished, "junk", "nope")},
			want:    []rowView{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := viewRows(buildPeerRows(tc.peers, tc.saved, tc.overlays, tc.running))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d rows %+v, want %+v", len(got), got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("row %d:\n got %+v\nwant %+v", i, got[i], tc.want[i])
				}
			}
		})
	}

	t.Run("relayed and last ping carried over", func(t *testing.T) {
		rows := buildPeerRows([]api.Peer{relayed}, nil, labOverlay, true)
		if len(rows) != 1 || !rows[0].Relayed || rows[0].LastPingMs == nil || *rows[0].LastPingMs != rtt || rows[0].CertName != "bob" {
			t.Errorf("rows %+v", rows)
		}
		if rows[0].Saved() {
			t.Error("unsaved peer reported as saved")
		}
	})
}

func TestCheckNewPeer(t *testing.T) {
	running := &api.StatusResponse{State: api.StateRunning, OverlayIPs: []string{"192.168.100.20/24"}}
	saved := []savedPeer{{"192.168.100.10", "Alice's laptop"}}
	tests := []struct {
		name, peerName, ip string
		st                 *api.StatusResponse
		field, msg         string
	}{
		{"ok", "Bob", "192.168.100.11", running, "", ""},
		{"ok with spaces", "  Bob ", " 192.168.100.11 ", running, "", ""},
		{"no name", " ", "192.168.100.11", running, "name", "Give it a name."},
		{"long name", "This name is much longer than forty characters", "192.168.100.11", running, "name",
			"That name is too long. Names can be at most 40 characters."},
		{"line break", "a\nb", "192.168.100.11", running, "name", "Names can't contain line breaks or tabs."},
		{"not an IP", "Bob", "192.168.100", running, "ip",
			"That doesn't look like an address. It should look like 192.168.100.10."},
		{"outside", "Bob", "10.1.0.5", running, "ip",
			"That address isn't on your Nebula network. Addresses on your network start with 192.168.100."},
		{"own address", "Me", "192.168.100.20", running, "ip", "That's this computer."},
		{"already saved", "Alice", "192.168.100.10", running, "ip", "Already in your list as Alice's laptop."},
		{"already saved, mapped form", "Alice", "::ffff:192.168.100.10", running, "ip", "Already in your list as Alice's laptop."},
		{"no status: format only", "Bob", "10.1.0.5", nil, "", ""},
		{"no status: bad format", "Bob", "bob", nil, "ip",
			"That doesn't look like an address. It should look like 192.168.100.10."},
		{"no status: still checks saved", "Alice", "192.168.100.10", nil, "ip", "Already in your list as Alice's laptop."},
		{"status without networks", "Bob", "10.1.0.5", &api.StatusResponse{OverlayIPs: []string{}}, "", ""},
		{"example from the network", "Bob", "x", &api.StatusResponse{OverlayIPs: []string{"10.42.0.3/16"}}, "ip",
			"That doesn't look like an address. It should look like 10.42.0.10."},
		{"network not on an octet", "Bob", "10.1.32.5", &api.StatusResponse{OverlayIPs: []string{"10.1.16.3/20"}}, "ip",
			"That address isn't on your Nebula network. Addresses on your network are 10.1.16.0 to 10.1.31.255."},
		{"two networks", "Bob", "10.1.0.5", &api.StatusResponse{OverlayIPs: []string{"192.168.100.20/24", "fd00::20/64"}}, "ip",
			"That address isn't on your Nebula network. Addresses on your network are 192.168.100.0 to 192.168.100.255, or fd00:: to fd00::ffff:ffff:ffff:ffff."},
		{"IPv6 inside", "Bob", "fd00::11", &api.StatusResponse{OverlayIPs: []string{"192.168.100.20/24", "fd00::20/64"}}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkNewPeer(tc.peerName, tc.ip, tc.st, saved)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			var pe *peerInputError
			if !errors.As(err, &pe) {
				t.Fatalf("error %v (%T), want a peerInputError", err, err)
			}
			if pe.Field != tc.field || pe.Msg != tc.msg {
				t.Errorf("got %s: %q\nwant %s: %q", pe.Field, pe.Msg, tc.field, tc.msg)
			}
		})
	}
}

func TestLastAddr(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.100.20/24": "192.168.100.255",
		"10.1.16.3/20":      "10.1.31.255",
		"10.0.0.1/32":       "10.0.0.1",
		"fd00::1/120":       "fd00::ff",
	} {
		if got := lastAddr(netip.MustParsePrefix(in)).String(); got != want {
			t.Errorf("lastAddr(%s) = %s, want %s", in, got, want)
		}
	}
}
