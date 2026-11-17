// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix

package gui

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// testInstanceAddr is a socket in a directory that doesn't exist yet, with
// a path short enough for macOS's socket path limit.
func testInstanceAddr(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "inst")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, appName, "gui.sock")
}

func TestInstancePermissions(t *testing.T) {
	addr := testInstanceAddr(t)
	if err := os.MkdirAll(filepath.Dir(addr), 0o750); err != nil {
		t.Fatal(err)
	}
	runningInstance(t, addr)

	for path, want := range map[string]os.FileMode{filepath.Dir(addr): 0o700, addr: 0o600} {
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}

func TestInstanceReplacesStaleSocket(t *testing.T) {
	addr := testInstanceAddr(t)
	if err := os.MkdirAll(filepath.Dir(addr), 0o700); err != nil {
		t.Fatal(err)
	}
	// A socket nothing listens on, as a crashed copy leaves behind.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: addr, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	_ = stale.Close()

	in, err := claimInstance(addr, true)
	if err != nil {
		t.Fatalf("claim over a stale socket: %v", err)
	}
	_ = in.Close()
}

func TestDefaultInstanceAddr(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	got, err := defaultInstanceAddr()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "gui.sock" || filepath.Base(filepath.Dir(got)) != appName {
		t.Errorf("defaultInstanceAddr() = %q", got)
	}
}
