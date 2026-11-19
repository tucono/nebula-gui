// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNoticesPath(t *testing.T) {
	for _, tc := range []struct{ goos, exe, want string }{
		{"windows", filepath.FromSlash("C:/Program Files/nebula-gui/nebula-gui.exe"),
			filepath.FromSlash("C:/Program Files/nebula-gui/" + noticesFile)},
		{"darwin", "/Applications/nebula-gui.app/Contents/MacOS/nebula-gui",
			filepath.Join("/Applications/nebula-gui.app/Contents/Resources", noticesFile)},
		{"linux", "/usr/bin/nebula-gui", "/usr/share/doc/nebula-gui/" + noticesFile},
	} {
		if got := noticesPath(tc.goos, tc.exe); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.goos, got, tc.want)
		}
	}
}

func TestFileURL(t *testing.T) {
	for path, want := range map[string]string{
		"/usr/share/doc/nebula-gui/" + noticesFile:   "file:///usr/share/doc/nebula-gui/" + noticesFile,
		"C:/Program Files/nebula-gui/" + noticesFile: "file:///C:/Program%20Files/nebula-gui/" + noticesFile,
	} {
		if got := fileURL(path).String(); got != want {
			t.Errorf("%q: %q, want %q", path, got, want)
		}
	}
}

func TestAboutDialog(t *testing.T) {
	m, _ := testWindow(t, newFakeService())
	saved := installedNotices
	t.Cleanup(func() { installedNotices = saved })

	installedNotices = func() string { return filepath.Join(t.TempDir(), noticesFile) }
	m.openAbout()
	over := m.w.Canvas().Overlays().Top()
	if over == nil {
		t.Fatal("no dialog shown")
	}
	for _, text := range []string{"nebula-gui 0.2.0", aboutSummary, aboutAffiliate} {
		if label(t, over, text) == nil {
			t.Errorf("no label %q", text)
		}
	}
	// A development build has no notices file.
	if label(t, over, aboutNoNotices) == nil || button(t, over, "Open-source licenses") != nil {
		t.Error("should say the licenses come with the installed app")
	}

	path := filepath.Join(t.TempDir(), noticesFile)
	if err := os.WriteFile(path, []byte("notices"), 0o600); err != nil {
		t.Fatal(err)
	}
	installedNotices = func() string { return path }
	m.openAbout()
	over = m.w.Canvas().Overlays().Top()
	if button(t, over, "Open-source licenses") == nil {
		t.Error("no button for the installed licenses")
	}
}
