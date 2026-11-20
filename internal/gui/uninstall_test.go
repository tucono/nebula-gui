// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"testing"
)

// helpLabels lists the Help menu's items, "-" for a separator.
func helpLabels(m *mainWindow) []string {
	var out []string
	for _, it := range m.mainMenu().Items[1].Items {
		if it.IsSeparator {
			out = append(out, "-")
		} else {
			out = append(out, it.Label)
		}
	}
	return out
}

func TestUninstallMenuItem(t *testing.T) {
	m, _ := testWindow(t, newFakeService())
	m.uninstaller = ""
	if got := helpLabels(m); len(got) != 2 {
		t.Fatalf("Help menu without an uninstaller: %q", got)
	}

	m.uninstaller = "/Applications/Uninstall nebula-gui.app"
	got := helpLabels(m)
	if len(got) != 4 || got[2] != "-" || got[3] != "Uninstall nebula-gui…" {
		t.Fatalf("Help menu with an uninstaller: %q", got)
	}

	var opened string
	quits := 0
	m.exit = func() { quits++ }
	m.runUninstaller = func(app string) error {
		opened = app
		return errors.New("couldn't open it")
	}
	m.mainMenu().Items[1].Items[3].Action()
	if opened != m.uninstaller || quits != 0 {
		t.Fatalf("after a failed open: opened %q, quit %d times", opened, quits)
	}

	m.runUninstaller = func(string) error { return nil }
	m.mainMenu().Items[1].Items[3].Action()
	if quits != 1 {
		t.Fatalf("quit %d times after opening the uninstaller, want 1", quits)
	}
}
