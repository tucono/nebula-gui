// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestDescribePing(t *testing.T) {
	avg := 23.4
	est := apiPeer(api.PeerEstablished, "alice", "192.168.100.10")
	pend := apiPeer(api.PeerPending, "", "192.168.100.10")
	replies := func(got int) *api.PingResponse {
		r := &api.PingResponse{VpnIP: "192.168.100.10", Sent: 3, Received: got}
		if got > 0 {
			r.RTTAvgMs = &avg
		}
		return r
	}
	for _, tc := range []struct {
		name          string
		before, after *api.Peer
		res           *api.PingResponse
		err           error
		want          pingView
		problem       problemID
	}{
		{"all replies", &est, &est, replies(3), nil, pingView{Kind: pingAnswered, Text: "Answered in 23 ms."}, ""},
		{"some lost", &est, &est, replies(2), nil, pingView{Kind: pingAnswered, Text: "Answered in 23 ms, 2 of 3 replies."}, ""},
		{"replies opened a tunnel", nil, &est, replies(1), nil, pingView{Kind: pingAnswered, Text: "Answered in 23 ms, 1 of 3 replies."}, ""},
		{"opened a pending tunnel", nil, &pend, replies(0), nil, pingView{Kind: pingWaiting, Text: pingTextConnecting, Retry: true}, ""},
		{"opened a tunnel", nil, &est, replies(0), nil, pingView{Kind: pingWaiting, Text: pingTextConnecting, Retry: true}, ""},
		{"retry, connected", &pend, &est, replies(0), nil, pingView{Kind: pingNoAnswer, Text: pingTextFirewall}, ""},
		{"connected all along", &est, &est, replies(0), nil, pingView{Kind: pingNoAnswer, Text: pingTextFirewall}, ""},
		{"retry, still pending", &pend, &pend, replies(0), nil, pingView{Kind: pingNoAnswer, Text: pingTextNoAnswer}, ""},
		{"no tunnel", nil, nil, replies(0), nil, pingView{Kind: pingNoAnswer, Text: pingTextNoAnswer}, ""},
		{"tunnel went away", &est, nil, replies(0), nil, pingView{Kind: pingNoAnswer, Text: pingTextNoAnswer}, ""},
		{"rate limited", nil, nil, nil, &api.Error{Code: api.CodeRateLimited, Message: "too many"}, pingView{Kind: pingWaiting, Text: pingTextBusy}, ""},
		{"ping failed", nil, nil, nil, &api.Error{Code: api.CodePingFailed, Message: "socket"}, pingView{Kind: pingError, Text: "Couldn't ping"}, probPingFailed},
		{"own address", nil, nil, nil, &api.Error{Code: api.CodeBadRequest, Reason: api.ReasonOwnAddress}, pingView{Kind: pingError, Text: "That's this computer"}, probOwnAddress},
		{"service down", nil, nil, nil, errRefused, pingView{Kind: pingError, Text: "The background service isn't running"}, probServiceNotRunning},
	} {
		got := describePing(tc.before, tc.after, tc.res, tc.err)
		var id problemID
		if got.Problem != nil {
			id = got.Problem.ID
		}
		if id != tc.problem {
			t.Errorf("%s: problem %q, want %q", tc.name, id, tc.problem)
		}
		got.Problem = nil
		if got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func pings(calls []string) []string {
	out := []string{}
	for _, c := range calls {
		if strings.HasPrefix(c, "ping ") {
			out = append(out, c)
		}
	}
	return out
}

// noNamesInLog fails if a saved name is in the GUI log.
func noNamesInLog(t *testing.T, m *mainWindow, names ...string) {
	t.Helper()
	for _, ev := range m.log.Recent(1000) {
		l := fmt.Sprint(ev)
		for _, n := range names {
			if strings.Contains(l, n) {
				t.Errorf("%q in the GUI log: %q", n, l)
			}
		}
	}
}

func TestPingConnectedPeer(t *testing.T) {
	m, svc, _ := peersWindow(t)
	svc.takeCalls()

	menuItem(t, m.peerMenu(rowFor(t, m, "alice")), "Ping").Action()
	if got := pings(svc.takeCalls()); !slices.Equal(got, []string{"ping 192.168.100.10"}) {
		t.Errorf("pings %v", got)
	}
	if m.peers.result.Hidden || m.peers.result.Text != "alice: Answered in 12 ms." {
		t.Errorf("result %q hidden %v", m.peers.result.Text, m.peers.result.Hidden)
	}
	if len(m.peers.pinging) != 0 {
		t.Errorf("still pinging %v", m.peers.pinging)
	}
	if !slices.Contains(msgs(m.log), "ping") {
		t.Errorf("log %v", msgs(m.log))
	}

	// The firewall case: connected, no replies.
	svc.set(func(f *fakeService) {
		f.ping = func(_ *fakeService, ip string) (*api.PingResponse, error) {
			return &api.PingResponse{VpnIP: ip, Sent: 3}, nil
		}
	})
	m.pingPeer(netip.MustParseAddr("192.168.100.10"), "alice")
	if m.peers.result.Text != "alice: "+pingTextFirewall {
		t.Errorf("result %q", m.peers.result.Text)
	}
	if got := pings(svc.takeCalls()); len(got) != 1 {
		t.Errorf("pings %v, want no retry", got)
	}
}

func TestPingOpensTunnelAndRetries(t *testing.T) {
	m, svc, _ := peersWindow(t)
	svc.takeCalls()
	// Bob has no tunnel. The first ping starts a handshake and gets no
	// replies; by the second the tunnel is up.
	svc.set(func(f *fakeService) {
		f.ping = func(f *fakeService, ip string) (*api.PingResponse, error) {
			if findPeer(f.peers, netip.MustParseAddr(ip)) == nil {
				f.peers = append(f.peers, apiPeer(api.PeerPending, "", ip))
				return &api.PingResponse{VpnIP: ip, Sent: 3}, nil
			}
			f.peers[len(f.peers)-1] = apiPeer(api.PeerEstablished, "bob", ip)
			return fakePing(f, ip)
		}
	})
	menuItem(t, m.peerMenu(rowFor(t, m, "Bob's PC")), "Ping").Action()
	if got := pings(svc.takeCalls()); !slices.Equal(got, []string{"ping 192.168.100.11", "ping 192.168.100.11"}) {
		t.Errorf("pings %v", got)
	}
	if m.peers.result.Text != "Bob's PC: Answered in 12 ms." {
		t.Errorf("result %q", m.peers.result.Text)
	}
	// The poke reads peers again, so Bob shows as connected.
	r, _ := m.poller.poll(t.Context())
	m.applyPoll(r)
	if got := peerStateText(rowFor(t, m, "Bob's PC")); got != "Connected · direct" {
		t.Errorf("Bob %q", got)
	}
	noNamesInLog(t, m, "Bob's PC")
}

// TestPingNobodyThere is an address nobody has: Nebula keeps a pending
// tunnel while it asks the lighthouse, so the first ping is retried, and
// the retry ends in No answer.
func TestPingNobodyThere(t *testing.T) {
	m, svc, _ := peersWindow(t)
	svc.takeCalls()
	svc.set(func(f *fakeService) {
		f.ping = func(f *fakeService, ip string) (*api.PingResponse, error) {
			if findPeer(f.peers, netip.MustParseAddr(ip)) == nil {
				f.peers = append(f.peers, apiPeer(api.PeerPending, "", ip))
			}
			return &api.PingResponse{VpnIP: ip, Sent: 3}, nil
		}
	})
	m.pingPeer(netip.MustParseAddr("192.168.100.99"), "Ghost")
	if got := pings(svc.takeCalls()); len(got) != 2 {
		t.Errorf("pings %v, want a retry", got)
	}
	if m.peers.result.Text != "Ghost: "+pingTextNoAnswer {
		t.Errorf("result %q", m.peers.result.Text)
	}
}

func TestPingNoAnswer(t *testing.T) {
	m, svc, _ := peersWindow(t)
	svc.takeCalls()
	m.pingPeer(netip.MustParseAddr("192.168.100.11"), "Bob's PC")
	if got := pings(svc.takeCalls()); len(got) != 1 {
		t.Errorf("pings %v, want one", got)
	}
	if m.peers.result.Text != "Bob's PC: "+pingTextNoAnswer {
		t.Errorf("result %q", m.peers.result.Text)
	}
}

func TestPingBusyAndErrors(t *testing.T) {
	m, svc, poll := peersWindow(t)
	alice := netip.MustParseAddr("192.168.100.10")

	svc.set(func(f *fakeService) {
		f.ping = func(*fakeService, string) (*api.PingResponse, error) {
			return nil, &api.Error{Code: api.CodeRateLimited, Message: "too many pings"}
		}
	})
	m.pingPeer(alice, "alice")
	if m.peers.result.Text != "alice: "+pingTextBusy {
		t.Errorf("result %q", m.peers.result.Text)
	}
	if o := m.w.Canvas().Overlays().Top(); o != nil {
		t.Errorf("dialog for a busy service: %T", o)
	}

	svc.set(func(f *fakeService) {
		f.ping = func(*fakeService, string) (*api.PingResponse, error) {
			return nil, &api.Error{Code: api.CodePingFailed, Message: "socket: operation not permitted"}
		}
	})
	m.pingPeer(alice, "alice")
	d := topDialog(t, m)
	if label(t, d, "Couldn't ping") == nil {
		t.Error("no problem dialog")
	}
	test.Tap(button(t, d, "Close"))
	if o := m.w.Canvas().Overlays().Top(); o != nil {
		t.Errorf("dialog still open: %T", o)
	}

	// A ping while one is running for the same peer does nothing, and its
	// menu item is disabled.
	m.peers.setPinging(alice, true)
	svc.takeCalls()
	if !menuItem(t, m.peerMenu(rowFor(t, m, "alice")), "Ping").Disabled {
		t.Error("Ping enabled while pinging")
	}
	m.pingPeer(alice, "alice")
	if got := pings(svc.takeCalls()); len(got) != 0 {
		t.Errorf("pings %v", got)
	}
	m.peers.setPinging(alice, false)

	// Disconnected: no Ping in the menu, and the old result goes away.
	svc.set(func(f *fakeService) { f.state = api.StateStopped })
	poll()
	if slices.Contains(menuLabels(m.peerMenu(rowFor(t, m, "Bob's PC"))), "Ping") {
		t.Error("Ping offered while disconnected")
	}
	if !m.peers.result.Hidden {
		t.Error("result still shown after disconnecting")
	}
}
