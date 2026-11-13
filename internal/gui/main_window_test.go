// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

// testWindow returns a window whose actions run in place, and a poll
// function that delivers one poll to it.
func testWindow(t *testing.T, svc *fakeService) (*mainWindow, func()) {
	t.Helper()
	a := test.NewTempApp(t)
	m := newMainWindow(a, svc, newAppLog(), "0.2.0")
	m.spawn = func(f func()) { f() }
	m.nativeOpen = nil // never open the desktop's real picker from a test
	t.Cleanup(m.w.Close)
	return m, func() {
		r, _ := m.poller.poll(context.Background())
		m.applyPoll(r)
	}
}

func TestMainWindowConnectDisconnect(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	root := m.w.Content()
	if !m.waiting.Visible() || m.header.Visible() {
		t.Fatal("should be waiting for the service")
	}

	poll()
	if m.waiting.Visible() || !m.header.Visible() || m.banner.Problem() != nil {
		t.Fatal("header not shown after the first status")
	}
	if m.headline.Text != "Not connected" || label(t, root, "192.168.100.20/24") == nil {
		t.Fatalf("headline %q", m.headline.Text)
	}
	if m.footer.String() != "App 0.2.0 · Service 0.2.0 · Nebula 1.12.0 · API 1" {
		t.Fatalf("footer %q", m.footer.String())
	}

	test.Tap(m.button)
	if m.headline.Text != "Connected" || m.button.Text != "Disconnect" || m.button.Disabled() {
		t.Fatalf("after Connect: %q, %q", m.headline.Text, m.button.Text)
	}
	test.Tap(m.button)
	if m.headline.Text != "Not connected" {
		t.Fatalf("after Disconnect: %q", m.headline.Text)
	}
	calls := svc.takeCalls()
	if calls[len(calls)-2] != "connect" || calls[len(calls)-1] != "disconnect" {
		t.Fatalf("calls %v", calls)
	}
}

func TestMainWindowConnectFails(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	poll()

	svc.set(func(f *fakeService) {
		f.configErr = &api.Error{Code: api.CodeInternal, Reason: api.ReasonTunUnavailable, Message: "Failed to get a tun/tap device"}
	})
	test.Tap(m.button)
	p := m.banner.Problem()
	if p == nil || p.ID != probTunUnavailable || !p.Dismiss {
		t.Fatalf("banner %+v", p)
	}
	if !m.header.Visible() || m.button.Disabled() {
		t.Fatal("header should stay usable while the service is stopped")
	}
	// The next poll doesn't clear it…
	poll()
	if m.banner.Problem() == nil {
		t.Fatal("poll cleared the failed action")
	}
	// …Dismiss does.
	test.Tap(button(t, m.w.Content(), "Dismiss"))
	if m.banner.Problem() != nil {
		t.Fatalf("still shown after Dismiss: %+v", m.banner.Problem())
	}

	var logged []string
	for _, e := range m.log.Recent(20) {
		logged = append(logged, e.Msg)
	}
	want := []string{"service reachable", "state changed", "connect clicked", "connect failed", "problem shown", "problem dismissed"}
	if !equal(logged, want) {
		t.Fatalf("log %v, want %v", logged, want)
	}
}

func TestMainWindowErrorState(t *testing.T) {
	svc := newFakeService()
	svc.state = api.StateError
	m, poll := testWindow(t, svc)
	poll()
	if m.header.Visible() || m.banner.Problem() == nil || m.banner.Problem().ID != probConnectFailed {
		t.Fatalf("error state: header %v, banner %+v", m.header.Visible(), m.banner.Problem())
	}
	// Dismiss clears the service's error state with Disconnect.
	test.Tap(button(t, m.w.Content(), "Dismiss"))
	if m.banner.Problem() != nil || m.headline.Text != "Not connected" {
		t.Fatalf("after Dismiss: banner %+v, headline %q", m.banner.Problem(), m.headline.Text)
	}
}

func TestMainWindowServiceDown(t *testing.T) {
	svc := newFakeService()
	svc.down = true
	m, poll := testWindow(t, svc)
	poll()
	if p := m.banner.Problem(); p == nil || p.ID != probServiceNotRunning {
		t.Fatalf("banner %+v", p)
	}
	if m.header.Visible() || m.waiting.Visible() {
		t.Fatal("header or waiting shown with the service down")
	}
	svc.set(func(f *fakeService) { f.down = false })
	poll()
	if m.banner.Problem() != nil || !m.header.Visible() {
		t.Fatal("banner didn't clear when the service came back")
	}
}
