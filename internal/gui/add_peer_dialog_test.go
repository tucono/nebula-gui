// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestAddPeerChecks(t *testing.T) {
	m, _, _ := peersWindow(t)
	test.Tap(m.peers.addB)
	a := m.addPeerD
	if a == nil {
		t.Fatal("dialog not open")
	}
	if !a.msg.Hidden || !a.add.Disabled() || !a.addPing.Disabled() {
		t.Error("an empty dialog should show no message and have its buttons disabled")
	}
	if a.ip.PlaceHolder != "For example, 192.168.100.10" {
		t.Errorf("placeholder %q", a.ip.PlaceHolder)
	}

	a.name.SetText("Carol")
	for ip, want := range map[string]string{
		"192.168.100":    "That doesn't look like an address. It should look like 192.168.100.10.",
		"10.0.0.5":       "That address isn't on your Nebula network. Addresses on your network start with 192.168.100.",
		"192.168.100.20": "That's this computer.",
		"192.168.100.11": "Already in your list as Bob's PC.",
	} {
		a.ip.SetText(ip)
		if a.msg.Hidden || a.msg.Text != want {
			t.Errorf("%s: message %q (hidden %v), want %q", ip, a.msg.Text, a.msg.Hidden, want)
		}
		if !a.add.Disabled() {
			t.Errorf("%s: Add enabled", ip)
		}
	}

	a.ip.SetText("192.168.100.12")
	if !a.msg.Hidden || a.add.Disabled() || a.addPing.Disabled() {
		t.Errorf("valid input: message %q, add disabled %v, add and ping disabled %v", a.msg.Text, a.add.Disabled(), a.addPing.Disabled())
	}
	a.name.SetText(" ")
	if a.msg.Text != "Give it a name." || !a.add.Disabled() {
		t.Errorf("empty name: %q", a.msg.Text)
	}

	// The dialog never grows past its width.
	if w := a.d.MinSize().Width; w > addPeerWidth {
		t.Errorf("min width %v", w)
	}

	// Escape cancels without saving.
	a.name.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if m.addPeerD != nil || m.w.Canvas().Overlays().Top() != nil {
		t.Error("Escape didn't close the dialog")
	}
	if len(m.saved.Peers()) != 1 {
		t.Errorf("saved %+v", m.saved.Peers())
	}
}

func TestAddPeerAndPing(t *testing.T) {
	m, svc, _ := peersWindow(t)
	svc.takeCalls()
	m.showAddPeer()
	a := m.addPeerD
	a.name.SetText("Carol's laptop")
	a.ip.SetText("192.168.100.12")
	test.Tap(a.addPing)

	if m.addPeerD != nil {
		t.Error("dialog still open")
	}
	if !slices.Contains(m.saved.Peers(), savedPeer{"192.168.100.12", "Carol's laptop"}) {
		t.Errorf("saved %+v", m.saved.Peers())
	}
	if got := pings(svc.takeCalls()); !slices.Equal(got, []string{"ping 192.168.100.12"}) {
		t.Errorf("pings %v", got)
	}
	if m.peers.result.Text != "Carol's laptop: "+pingTextNoAnswer {
		t.Errorf("result %q", m.peers.result.Text)
	}
	rowFor(t, m, "Carol's laptop")
	if !slices.Contains(msgs(m.log), "peer added") {
		t.Errorf("log %v", msgs(m.log))
	}
	noNamesInLog(t, m, "Carol")
}

func TestAddPeerWhileDisconnected(t *testing.T) {
	m, svc, poll := peersWindow(t)
	svc.set(func(f *fakeService) { f.state = api.StateStopped })
	poll()
	svc.takeCalls()

	m.showAddPeer()
	a := m.addPeerD
	a.name.SetText("Carol")
	a.ip.SetText("192.168.100.12")
	if a.add.Disabled() || !a.addPing.Disabled() {
		t.Errorf("add disabled %v, add and ping disabled %v", a.add.Disabled(), a.addPing.Disabled())
	}
	// Enter adds without pinging.
	a.ip.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if m.addPeerD != nil {
		t.Error("Enter didn't close the dialog")
	}
	if got := titles(m.peers.rows); !slices.Contains(got, "Carol: Not connected") {
		t.Errorf("rows %v", got)
	}
	if got := pings(svc.takeCalls()); len(got) != 0 {
		t.Errorf("pings %v", got)
	}
}
