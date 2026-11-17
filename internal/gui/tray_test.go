// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestTrayModel(t *testing.T) {
	status := func(state api.State) *api.StatusResponse {
		return &api.StatusResponse{State: state, Configured: true, OverlayIPs: []string{"192.168.100.20/24"}}
	}
	failed := &problem{ID: "connect_failed"}
	cases := []struct {
		name string
		in   statusInput
		want trayView
	}{
		{"before the first answer", statusInput{},
			trayView{trayOff, "Not connected", actConnect, "Connect", false}},
		{"service unreachable", statusInput{Err: errRefused},
			trayView{trayAttention, needsAttention, actConnect, "Connect", false}},
		{"another API version", statusInput{Version: &problem{ID: "app_too_old"}},
			trayView{trayAttention, needsAttention, actConnect, "Connect", false}},
		{"no config", statusInput{Status: &api.StatusResponse{State: api.StateStopped}},
			trayView{trayOff, "Not connected · no config", actConnect, "Connect", false}},
		{"stopped", statusInput{Status: status(api.StateStopped)},
			trayView{trayOff, "Not connected", actConnect, "Connect", true}},
		{"stopped after a failed connect", statusInput{Status: status(api.StateStopped), Action: failed},
			trayView{trayAttention, needsAttention, actConnect, "Connect", true}},
		{"starting", statusInput{Status: status(api.StateStarting)},
			trayView{trayBusy, "Connecting…", actConnect, "Connect", false}},
		{"running", statusInput{Status: status(api.StateRunning)},
			trayView{trayOn, "Connected · 192.168.100.20", actDisconnect, "Disconnect", true}},
		{"running without an address yet", statusInput{Status: &api.StatusResponse{State: api.StateRunning, Configured: true}},
			trayView{trayOn, "Connected", actDisconnect, "Disconnect", true}},
		{"stopping", statusInput{Status: status(api.StateStopping)},
			trayView{trayBusy, "Disconnecting…", actDisconnect, "Disconnect", false}},
		{"error", statusInput{Status: status(api.StateError)},
			trayView{trayAttention, needsAttention, actConnect, "Connect", true}},
		{"connect clicked", statusInput{Status: status(api.StateStopped), Acting: actConnect},
			trayView{trayBusy, "Connecting…", actConnect, "Connect", false}},
		{"disconnect clicked", statusInput{Status: status(api.StateRunning), Acting: actDisconnect},
			trayView{trayBusy, "Disconnecting…", actDisconnect, "Disconnect", false}},
		{"removing the config", statusInput{Status: status(api.StateStopped), Acting: actRemoveConfig},
			trayView{trayBusy, "Removing the config…", actConnect, "Connect", false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := trayModel(c.in); got != c.want {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// fakeTray records what the window shows in the tray.
type fakeTray struct {
	menu    *fyne.Menu
	icon    fyne.Resource
	tooltip string
	tapped  func()
	menus   int
}

func (f *fakeTray) SetMenu(m *fyne.Menu)    { f.menu = m; f.menus++ }
func (f *fakeTray) SetIcon(r fyne.Resource) { f.icon = r }
func (f *fakeTray) SetTooltip(s string)     { f.tooltip = s }
func (f *fakeTray) SetOnTapped(fn func())   { f.tapped = fn }

// labels lists the menu's items, with "-" for a separator and a trailing
// " (off)" for a disabled item.
func (f *fakeTray) labels() string {
	var out []string
	for _, it := range f.menu.Items {
		switch {
		case it.IsSeparator:
			out = append(out, "-")
		case it.Disabled:
			out = append(out, it.Label+" (off)")
		default:
			out = append(out, it.Label)
		}
	}
	return strings.Join(out, " | ")
}

func (f *fakeTray) item(label string) *fyne.MenuItem {
	for _, it := range f.menu.Items {
		if it.Label == label {
			return it
		}
	}
	return nil
}

func trayWindow(t *testing.T, svc *fakeService) (*mainWindow, *fakeTray, func()) {
	t.Helper()
	m, poll := testWindow(t, svc)
	tray := &fakeTray{}
	m.attachTray(tray)
	return m, tray, poll
}

func TestMainWindowTray(t *testing.T) {
	svc := newFakeService()
	m, tray, poll := trayWindow(t, svc)
	if tray.labels() != "Not connected (off) | - | Connect (off) | Open nebula-gui | - | Quit" {
		t.Fatalf("before the first poll: %s", tray.labels())
	}

	poll()
	if tray.labels() != "Not connected (off) | - | Connect | Open nebula-gui | - | Quit" {
		t.Fatalf("stopped: %s", tray.labels())
	}
	if tray.icon != trayIcons[trayOff] || tray.tooltip != "Not connected" {
		t.Fatalf("stopped: icon %s, tooltip %q", tray.icon.Name(), tray.tooltip)
	}

	tray.item("Connect").Action()
	if tray.tooltip != "Connected · 192.168.100.20" || tray.icon != trayIcons[trayOn] {
		t.Fatalf("after Connect: icon %s, tooltip %q", tray.icon.Name(), tray.tooltip)
	}
	if tray.item("Disconnect") == nil || tray.item("Disconnect").Disabled {
		t.Fatalf("after Connect: %s", tray.labels())
	}
	if c := svc.takeCalls(); c[len(c)-1] != "connect" {
		t.Fatalf("calls %v", c)
	}

	// The header and the tray agree.
	if m.headline.Text != "Connected" {
		t.Fatalf("headline %q", m.headline.Text)
	}

	// The menu is set again only when it changes, after the first few.
	for range trayReapply {
		poll()
	}
	n := tray.menus
	poll()
	if tray.menus != n {
		t.Fatal("menu set again although nothing changed")
	}

	// A stale menu item does nothing: Connect while connected.
	m.trayAct(actConnect)
	if c := svc.takeCalls(); slices.Contains(c, "connect") {
		t.Fatalf("stale Connect ran: %v", c)
	}

	svc.set(func(f *fakeService) { f.down = true })
	poll()
	if tray.icon != trayIcons[trayAttention] || tray.tooltip != needsAttention {
		t.Fatalf("service down: icon %s, tooltip %q", tray.icon.Name(), tray.tooltip)
	}
}

func TestMainWindowTrayConnectFails(t *testing.T) {
	svc := newFakeService()
	m, tray, poll := trayWindow(t, svc)
	poll()
	m.tabs.SelectIndex(2)
	m.hideToTray()

	svc.set(func(f *fakeService) { f.configErr = &api.Error{Code: api.CodeInternal, Message: "bad"} })
	tray.item("Connect").Action()
	if m.hidden || m.tabs.SelectedIndex() != 0 {
		t.Fatalf("a failed Connect from the tray should show the Status tab: hidden %v, tab %d", m.hidden, m.tabs.SelectedIndex())
	}
	if m.banner.Problem() == nil {
		t.Fatal("no problem shown")
	}

	// From the window, a failure doesn't switch tabs.
	m.tabs.SelectIndex(2)
	m.in.Action = nil
	m.act(actConnect)
	if m.tabs.SelectedIndex() != 2 {
		t.Fatal("a failed Connect from the window switched tabs")
	}
}

func TestMainWindowHideToTray(t *testing.T) {
	svc := newFakeService()
	svc.set(func(f *fakeService) { f.state = api.StateRunning })
	m, tray, poll := trayWindow(t, svc)
	poll()
	svc.takeCalls()

	test.AssertNotificationSent(t, fyne.NewNotification("nebula-gui", "nebula-gui is still running in the tray."), m.hideToTray)
	if !m.hidden || !m.poller.hidden.Load() || !m.logPoller.hidden.Load() {
		t.Fatal("pollers not switched to hidden")
	}
	// Hidden: status only, no peers.
	poll()
	if c := svc.takeCalls(); !slices.Equal(c, []string{"status"}) {
		t.Fatalf("hidden poll calls %v", c)
	}

	// Once only.
	m.showWindow()
	test.AssertNotificationSent(t, nil, m.hideToTray)

	// A left click on the icon shows the window and reads the peers.
	tray.tapped()
	if m.hidden || m.poller.hidden.Load() || m.logPoller.hidden.Load() {
		t.Fatal("still hidden after a click on the icon")
	}
	<-m.poller.wake
	poll()
	if c := svc.takeCalls(); !slices.Contains(c, "peers") {
		t.Fatalf("shown poll calls %v", c)
	}
}

func TestMainWindowQuit(t *testing.T) {
	svc := newFakeService()
	m, _, poll := trayWindow(t, svc)
	quits := 0
	m.exit = func() { quits++ }
	poll()

	// Not connected: quits at once, and the notice is still to come.
	m.quit()
	if quits != 1 || m.a.Preferences().Bool(prefQuitNotice) {
		t.Fatalf("quits %d", quits)
	}

	// Connected, the first time: asks, saying the connection stays on.
	svc.set(func(f *fakeService) { f.state = api.StateRunning })
	poll()
	m.hideToTray()
	m.quit()
	if quits != 1 || m.hidden {
		t.Fatalf("quit without asking, or the window stayed hidden: quits %d", quits)
	}
	if label(t, m.w.Canvas().Overlays().Top(), "The connection stays on. Disconnect first if you want it off.") == nil {
		t.Fatal("no notice")
	}
	test.Tap(button(t, m.w.Canvas().Overlays().Top(), "Quit"))
	if quits != 2 {
		t.Fatalf("quits %d after confirming", quits)
	}

	// Then never again.
	m.quit()
	if quits != 3 {
		t.Fatalf("quits %d the second time", quits)
	}
}

func TestPollerHidden(t *testing.T) {
	svc := newFakeService()
	svc.set(func(f *fakeService) { f.state = api.StateRunning })
	p := newPoller(svc, newAppLog(), &statusGate{})
	ctx := context.Background()

	r, d := p.poll(ctx)
	if !r.PeersUpdated || p.delay(d) != pollInterval {
		t.Fatalf("shown: peers %v, delay %v", r.PeersUpdated, p.delay(d))
	}
	p.SetHidden(true)
	p.Poke() // a Poke while hidden still doesn't read peers
	<-p.wake
	svc.takeCalls()
	r, d = p.poll(ctx)
	if r.PeersUpdated || p.delay(d) != hiddenPollInterval {
		t.Fatalf("hidden: peers %v, delay %v", r.PeersUpdated, p.delay(d))
	}
	if c := svc.takeCalls(); slices.Contains(c, "peers") {
		t.Fatalf("hidden calls %v", c)
	}
	if p.delay(maxPollDelay) != hiddenPollInterval {
		t.Fatal("backoff while hidden")
	}

	// Showing polls at once, with peers.
	p.SetHidden(false)
	select {
	case <-p.wake:
	default:
		t.Fatal("showing didn't poke")
	}
	if r, _ = p.poll(ctx); !r.PeersUpdated {
		t.Fatal("no peers once shown")
	}
}

func TestLogPollerHidden(t *testing.T) {
	svc := newFakeService()
	p := newLogPoller(svc)
	p.SetHidden(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := make(chan logUpdate, 10)
	go p.run(ctx, func(u logUpdate) { updates <- u })

	select {
	case u := <-updates:
		t.Fatalf("read while hidden: %+v", u)
	case <-time.After(100 * time.Millisecond):
	}
	p.SetHidden(false)
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("showing didn't read")
	}
}
