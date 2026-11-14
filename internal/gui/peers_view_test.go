// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestPeerStateText(t *testing.T) {
	ms := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		row  peerRow
		want string
	}{
		{peerRow{Status: peerConnected}, "Connected · direct"},
		{peerRow{Status: peerConnected, Relayed: true}, "Connected · relayed"},
		{peerRow{Status: peerConnected, LastPingMs: ms(23.4)}, "Connected · direct · 23 ms"},
		{peerRow{Status: peerConnected, LastPingMs: ms(0.2)}, "Connected · direct · <1 ms"},
		{peerRow{Status: peerConnecting}, "Connecting…"},
		{peerRow{Status: peerNotConnected}, "Not connected"},
	} {
		if got := peerStateText(tc.row); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.row, got, tc.want)
		}
	}
}

// peersWindow returns a window connected to a fake service with alice
// (cert name only) and the lighthouse, and a saved peer bob who isn't
// connected.
func peersWindow(t *testing.T) (*mainWindow, *fakeService, func()) {
	t.Helper()
	svc := newFakeService()
	svc.state = api.StateRunning
	svc.peers = []api.Peer{
		apiPeer(api.PeerEstablished, "alice", "192.168.100.10"),
		apiPeer(api.PeerEstablished, "lighthouse", "192.168.100.1"),
	}
	m, poll := testWindow(t, svc)
	m.w.Resize(fyne.NewSize(520, 420))
	if err := m.saved.Add(netip.MustParseAddr("192.168.100.11"), "Bob's PC"); err != nil {
		t.Fatal(err)
	}
	poll()
	return m, svc, poll
}

func titles(rows []peerRow) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Title+": "+peerStateText(r))
	}
	return out
}

func menuLabels(items []*fyne.MenuItem) []string {
	out := []string{}
	for _, it := range items {
		if it.IsSeparator {
			out = append(out, "--")
			continue
		}
		out = append(out, it.Label)
	}
	return out
}

func menuItem(t *testing.T, items []*fyne.MenuItem, label string) *fyne.MenuItem {
	t.Helper()
	for _, it := range items {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("no %q in %v", label, menuLabels(items))
	return nil
}

func rowFor(t *testing.T, m *mainWindow, title string) peerRow {
	t.Helper()
	for _, r := range m.peers.rows {
		if r.Title == title {
			return r
		}
	}
	t.Fatalf("no row %q in %v", title, titles(m.peers.rows))
	return peerRow{}
}

func TestPeersTab(t *testing.T) {
	m, svc, poll := peersWindow(t)

	want := []string{"alice: Connected · direct", "lighthouse: Connected · direct", "Bob's PC: Not connected"}
	if got := titles(m.peers.rows); !slices.Equal(got, want) {
		t.Fatalf("rows %v", got)
	}
	if m.peers.note.Text != peersNoteRunning || m.peers.list.Hidden || !m.peers.empty.Hidden {
		t.Errorf("note %q, list hidden %v, empty hidden %v", m.peers.note.Text, m.peers.list.Hidden, m.peers.empty.Hidden)
	}

	// Selecting the tab reads peers on every poll; leaving it slows down.
	m.tabs.SelectIndex(1)
	if !m.poller.peersFast.Load() || !m.peersShowing() {
		t.Error("peers not fast while the tab shows")
	}
	m.tabs.SelectIndex(0)
	if m.poller.peersFast.Load() {
		t.Error("peers still fast on the Status tab")
	}

	// Disconnected: only saved peers, and a note to connect.
	svc.set(func(f *fakeService) { f.state = api.StateStopped })
	poll()
	if got := titles(m.peers.rows); !slices.Equal(got, []string{"Bob's PC: Not connected"}) {
		t.Errorf("rows when stopped %v", got)
	}
	if m.peers.note.Text != peersNoteStopped {
		t.Errorf("note %q", m.peers.note.Text)
	}

	// Nothing at all: the empty state.
	if err := m.saved.Remove(netip.MustParseAddr("192.168.100.11")); err != nil {
		t.Fatal(err)
	}
	m.render()
	if !m.peers.list.Hidden || m.peers.empty.Hidden {
		t.Error("empty state not shown")
	}
}

func TestPeerMenu(t *testing.T) {
	m, _, _ := peersWindow(t)
	for title, want := range map[string][]string{
		"alice":    {"Copy IP", "Give it a name…", "--", "Close connection"},
		"Bob's PC": {"Copy IP", "Rename…", "Remove from list"},
	} {
		if got := menuLabels(m.peerMenu(rowFor(t, m, title))); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", title, got, want)
		}
	}

	// The ⋯ button opens the menu over the window.
	m.showPeerMenu(rowFor(t, m, "alice"), widget.NewButton("", nil))
	topDialog(t, m)
	closeDialogs(m)
}

func TestPeerCopyIP(t *testing.T) {
	m, _, _ := peersWindow(t)
	menuItem(t, m.peerMenu(rowFor(t, m, "alice")), "Copy IP").Action()
	if got := fyne.CurrentApp().Clipboard().Content(); got != "192.168.100.10" {
		t.Errorf("clipboard %q", got)
	}
	if m.peers.copied != netip.MustParseAddr("192.168.100.10") {
		t.Errorf("row showing Copied: %v", m.peers.copied)
	}
}

func TestPeerRenameAndRemove(t *testing.T) {
	m, svc, _ := peersWindow(t)

	// Give alice a name through the dialog; it starts from her cert name.
	menuItem(t, m.peerMenu(rowFor(t, m, "alice")), "Give it a name…").Action()
	d := topDialog(t, m)
	e, ok := find(t, d, func(e *dialogEntry) bool { return true })
	if !ok || e.Text != "alice" {
		t.Fatalf("entry %v, text %q", ok, e.Text)
	}
	e.SetText("alice's laptop")
	test.Tap(button(t, d, "Save"))
	if m.w.Canvas().Overlays().Top() != nil {
		t.Error("dialog still open after Save")
	}
	r := rowFor(t, m, "alice's laptop")
	if r.Subtitle != "192.168.100.10 · alice" || !r.Saved() {
		t.Errorf("after naming %+v", r)
	}
	if !slices.Equal(m.saved.Peers(), []savedPeer{{"192.168.100.11", "Bob's PC"}, {"192.168.100.10", "alice's laptop"}}) {
		t.Errorf("saved %+v", m.saved.Peers())
	}

	// An empty name can't be saved.
	menuItem(t, m.peerMenu(rowFor(t, m, "Bob's PC")), "Rename…").Action()
	d = topDialog(t, m)
	e, _ = find(t, d, func(e *dialogEntry) bool { return true })
	e.SetText("  ")
	if e.Validate() == nil || e.Validate().Error() != "Give it a name." {
		t.Errorf("validate %v", e.Validate())
	}
	// Enter does nothing while the name is invalid.
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if label(t, topDialog(t, m), "Rename") == nil {
		t.Fatal("Rename dialog not on top after Enter with an invalid name")
	}
	// Escape cancels.
	e.SetText("Robert")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if m.w.Canvas().Overlays().Top() != nil {
		t.Fatalf("Escape didn't close the dialog: top %T", m.w.Canvas().Overlays().Top())
	}
	rowFor(t, m, "Bob's PC")

	// Enter saves.
	menuItem(t, m.peerMenu(rowFor(t, m, "Bob's PC")), "Rename…").Action()
	e, _ = find(t, topDialog(t, m), func(e *dialogEntry) bool { return true })
	e.SetText("Bob")
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if m.w.Canvas().Overlays().Top() != nil {
		t.Error("Enter didn't close the dialog")
	}
	rowFor(t, m, "Bob")

	// Removing alice's name leaves her under her cert name; removing Bob
	// takes him off the list.
	menuItem(t, m.peerMenu(rowFor(t, m, "alice's laptop")), "Remove from list").Action()
	menuItem(t, m.peerMenu(rowFor(t, m, "Bob")), "Remove from list").Action()
	if got := titles(m.peers.rows); !slices.Equal(got, []string{"alice: Connected · direct", "lighthouse: Connected · direct"}) {
		t.Errorf("after removing %v", got)
	}
	if len(m.saved.Peers()) != 0 {
		t.Errorf("saved %+v", m.saved.Peers())
	}
	// None of it touched the service.
	for _, c := range svc.takeCalls() {
		if strings.HasPrefix(c, "close") {
			t.Errorf("service call %q", c)
		}
	}
	// The GUI log never has a saved name.
	for _, ev := range m.log.Recent(1000) {
		l := fmt.Sprint(ev)
		if strings.Contains(l, "laptop") || strings.Contains(l, "Bob") {
			t.Errorf("name in the GUI log: %q", l)
		}
	}
}

func TestPeerClose(t *testing.T) {
	m, svc, poll := peersWindow(t)
	svc.takeCalls()

	menuItem(t, m.peerMenu(rowFor(t, m, "alice")), "Close connection").Action()
	d := topDialog(t, m)
	if label(t, d, "Close the connection to alice? It reconnects the next time either computer sends something.") == nil {
		t.Error("confirmation text")
	}
	test.Tap(button(t, d, "Close connection"))
	if calls := svc.takeCalls(); !slices.Contains(calls, "close 192.168.100.10") {
		t.Fatalf("calls %v", calls)
	}
	poll()
	if got := titles(m.peers.rows); !slices.Equal(got, []string{"lighthouse: Connected · direct", "Bob's PC: Not connected"}) {
		t.Errorf("after close %v", got)
	}

	// A tunnel that's already gone is fine: no error shown.
	m.closePeer(netip.MustParseAddr("192.168.100.10"))
	if o := m.w.Canvas().Overlays().Top(); o != nil {
		t.Errorf("dialog after closing a closed tunnel: %T", o)
	}

	// Other errors are shown.
	svc.set(func(f *fakeService) { f.closeErr = &api.Error{Code: api.CodeInternal, Message: "boom"} })
	m.closePeer(netip.MustParseAddr("192.168.100.1"))
	if label(t, topDialog(t, m), "Couldn't close the connection") == nil {
		t.Error("no error dialog")
	}
}
