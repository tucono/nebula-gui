// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

func (f *fakeService) Diagnostics(context.Context, int) (*api.DiagnosticsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "diagnostics")
	if f.down {
		return nil, errRefused
	}
	return &api.DiagnosticsResponse{
		ServiceVersion: "0.2.0", NebulaVersion: "1.12.0", APIVersion: f.apiVer,
		InstanceID: f.instance, OS: "linux", Arch: "amd64",
		Status: *f.statusLocked(),
		ConfigSummary: &api.ConfigSummary{
			Lighthouses:     []string{"lighthouse.example.net:4242"},
			StaticHostNames: []string{"lighthouse.example.net"},
		},
		Log: []string{`{"time":"2026-11-12T14:03:20Z","level":"INFO","msg":"Handshake message received","component":"nebula","certName":"alice-laptop","udpAddr":"203.0.113.10:4242","vpnAddrs":["192.168.100.31"]}`},
	}, nil
}

func TestMainWindowGetHelp(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	poll()

	m.openHelp(nil)
	h := m.help
	if h == nil || !h.ready {
		t.Fatal("dialog not open, or the diagnostics weren't read")
	}
	text := h.preview.Text
	for _, s := range []string{"# Nebula troubleshooting report", "Masking: on", "OVERLAY-1/24", `"certName":"HOST-`, "PUBLIC-1:4242"} {
		if !strings.Contains(text, s) {
			t.Errorf("preview lacks %q", s)
		}
	}
	for _, s := range []string{"alice-laptop", "203.0.113.10", "192.168.100.", "## Problem"} {
		if strings.Contains(text, s) {
			t.Errorf("preview has %q", s)
		}
	}
	// The diagnostics' names are remembered for the next report.
	if n := m.names.Names(); !equal(n.Hosts, []string{"lighthouse.example.net"}) || !equal(n.Overlays, []string{"192.168.100.0/24"}) {
		t.Errorf("remembered %+v", n)
	}

	test.Tap(h.copyB)
	if got := fyne.CurrentApp().Clipboard().Content(); got != text || h.copyB.Text != "Copied" {
		t.Errorf("clipboard %d bytes, button %q", len(got), h.copyB.Text)
	}

	test.Tap(h.details)
	if !strings.Contains(h.preview.Text, "Masking: off") || !strings.Contains(h.preview.Text, "alice-laptop") {
		t.Error("network details not included")
	}
	if h.copyB.Text != "Copy" {
		t.Error("Copied stayed after the report changed")
	}

	// Save as file… writes the report shown, readable only by the user.
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	var asked openRequest
	m.nativeOpen = func(_ context.Context, _ string, req openRequest) (string, error) {
		asked = req
		return path, nil
	}
	test.Tap(h.saveB)
	if !asked.Save || !strings.HasPrefix(asked.FileName, "nebula-report-") {
		t.Errorf("asked %+v", asked)
	}
	b, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil || string(b) != h.text {
		t.Fatalf("saved %d bytes, err %v", len(b), err)
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}

	m.openHelp(nil) // already open: nothing happens
	if m.help != h {
		t.Error("a second dialog opened")
	}
	h.d.Hide()
	if m.help != nil {
		t.Error("dialog still recorded after closing")
	}
	if !slices.Contains(msgs(m.log), "get help opened") || !slices.Contains(msgs(m.log), "report saved") {
		t.Errorf("log %v", msgs(m.log))
	}
}

func TestMainWindowGetHelpUnreachable(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	poll()
	svc.set(func(f *fakeService) { f.down = true })
	poll()

	p := m.bannerProblem()
	if p == nil || p.ID != probServiceNotRunning {
		t.Fatalf("banner %+v", p)
	}
	m.openHelp(p)
	text := m.help.preview.Text
	for _, s := range []string{"## Problem", "[service_not_running]", "Not available: the background service couldn't be reached."} {
		if !strings.Contains(text, s) {
			t.Errorf("preview lacks %q", s)
		}
	}
	if (runtime.GOOS == "linux") != strings.Contains(text, "journalctl -u nebula-guid") {
		t.Error("journalctl hint wrong for this OS")
	}
}

func TestMainWindowHelpEntries(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	poll()

	// Help → Get help… in the menu.
	menu := m.w.MainMenu()
	if menu == nil || len(menu.Items) != 2 || menu.Items[1].Label != "Help" ||
		menu.Items[1].Items[0].Label != "Get help…" || !menu.Items[0].Items[0].IsQuit {
		t.Fatal("menu missing")
	}
	menu.Items[1].Items[0].Action()
	if m.help == nil {
		t.Fatal("menu didn't open Get help")
	}
	m.help.d.Hide()

	// Get help… at the top of the Logs tab.
	if b := button(t, m.logs.root, "Get help…"); b == nil {
		t.Fatal("no Get help… on the Logs tab")
	} else {
		test.Tap(b)
	}
	if m.help == nil {
		t.Fatal("Logs tab didn't open Get help")
	}
	m.help.d.Hide()

	// Get help… in the problem banner.
	svc.set(func(f *fakeService) { f.down = true })
	poll()
	b := button(t, m.banner.Object(), "Get help…")
	if b == nil {
		t.Fatal("no Get help… in the banner")
	}
	test.Tap(b)
	if m.help == nil || m.help.in.Problem == nil || m.help.in.Problem.ID != probServiceNotRunning {
		t.Fatal("banner didn't open Get help with its problem")
	}
}

func TestRemoveConfigForgetsNames(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	poll()
	if n := m.names.Names(); len(n.Overlays) != 1 {
		t.Fatalf("status names not remembered: %+v", n)
	}
	m.act(actRemoveConfig)
	if n := m.names.Names(); len(n.Overlays) != 0 {
		t.Errorf("names kept after removing the config: %+v", n)
	}
}
