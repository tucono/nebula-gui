//go:build linux && !android

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testEntry is the entry for exe under a temporary config directory.
func testEntry(t *testing.T, exe string) (Entry, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	e, err := New(exe)
	if err != nil {
		t.Fatal(err)
	}
	e.(*desktopFile).reload = nil // never reload the real user manager
	return e, filepath.Join(dir, "autostart", "nebula-gui.desktop")
}

func enabled(t *testing.T, e Entry) bool {
	t.Helper()
	on, err := e.Enabled()
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	return on
}

func TestDesktopFileOnAndOff(t *testing.T) {
	e, path := testEntry(t, "/usr/bin/nebula-gui")
	if enabled(t, e) {
		t.Fatal("enabled before it was set")
	}
	if err := e.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !enabled(t, e) {
		t.Fatal("not enabled after SetEnabled(true)")
	}
	b, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	want := "[Desktop Entry]\nType=Application\nName=nebula-gui\nComment=Starts nebula-gui in the tray\n" +
		"Exec=/usr/bin/nebula-gui --hidden\nTerminal=false\n"
	if string(b) != want {
		t.Fatalf("file:\n%s\nwant:\n%s", b, want)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v, want 0644", fi.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".nebula-gui-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}

	if err := e.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if enabled(t, e) {
		t.Fatal("still enabled after SetEnabled(false)")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if err := e.SetEnabled(false); err != nil {
		t.Fatalf("turning off twice: %v", err)
	}
}

func TestDesktopFileReloads(t *testing.T) {
	e, _ := testEntry(t, "/usr/bin/nebula-gui")
	reloads := 0
	e.(*desktopFile).reload = func() { reloads++ }
	for _, on := range []bool{true, false} {
		if err := e.SetEnabled(on); err != nil {
			t.Fatal(err)
		}
	}
	if reloads != 2 {
		t.Fatalf("%d reloads, want one per change", reloads)
	}
}

func TestDesktopFileTurnedOffInside(t *testing.T) {
	for _, line := range []string{"Hidden=true", "X-GNOME-Autostart-enabled=false"} {
		e, path := testEntry(t, "/usr/bin/nebula-gui")
		if err := e.SetEnabled(true); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(path)                                                  //nolint:gosec // test file
		if err := os.WriteFile(path, append(b, line+"\n"...), 0o600); err != nil { //nolint:gosec // test file
			t.Fatal(err)
		}
		if enabled(t, e) {
			t.Errorf("%s: still enabled", line)
		}
		if err := e.SetEnabled(true); err != nil {
			t.Fatal(err)
		}
		if !enabled(t, e) {
			t.Errorf("%s: turning it on again didn't", line)
		}
	}
}

func TestDesktopFileOtherGroupIgnored(t *testing.T) {
	e, path := testEntry(t, "/usr/bin/nebula-gui")
	data := "[Desktop Entry]\nExec=nebula-gui\n[Desktop Action x]\nHidden=true\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if !enabled(t, e) {
		t.Fatal("Hidden in another group turned it off")
	}
}

func TestExecArg(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/usr/bin/nebula-gui", "/usr/bin/nebula-gui"},
		{"/home/alice/My Apps/nebula-gui", `"/home/alice/My Apps/nebula-gui"`},
		{"/opt/100%/nebula-gui", "/opt/100%%/nebula-gui"},
		{`/opt/a"b/nebula-gui`, `"/opt/a\\"b/nebula-gui"`},
		{`/opt/$x/nebula-gui`, `"/opt/\\$x/nebula-gui"`},
		{`/opt/a\b/nebula-gui`, `"/opt/a\\\\b/nebula-gui"`},
	}
	for _, c := range cases {
		got, err := execArg(c.in)
		if err != nil || got != c.want {
			t.Errorf("execArg(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if _, err := execArg("/opt/a\nb"); err == nil {
		t.Error("a line break was accepted")
	}
}

func TestDesktopFileQuotesPath(t *testing.T) {
	e, path := testEntry(t, "/home/alice/My Apps/nebula-gui")
	if err := e.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path) //nolint:gosec // test file
	if !strings.Contains(string(b), "\nExec=\"/home/alice/My Apps/nebula-gui\" --hidden\n") {
		t.Fatalf("file:\n%s", b)
	}
}
