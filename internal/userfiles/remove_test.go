// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// fakeHome makes a home folder with all of the app's files, and another
// app's files next to them.
func fakeHome(t *testing.T) (home string, others []string) {
	t.Helper()
	home = t.TempDir()
	f := For(runtime.GOOS, home)
	for _, d := range f.Dirs {
		write(t, filepath.Join(d, "file.json"))
		write(t, filepath.Join(d, "logs", "app.log"))
	}
	if f.Autostart != "" {
		write(t, f.Autostart)
	}
	// Another app's files beside each of ours.
	for _, d := range f.Dirs {
		others = append(others, filepath.Join(filepath.Dir(d), "other-app", "keep.json"))
	}
	if f.Autostart != "" {
		others = append(others, filepath.Join(filepath.Dir(f.Autostart), "other-app.desktop"))
	}
	others = append(others, filepath.Join(home, "Documents", "nebula-gui-report.txt"))
	for _, p := range others {
		write(t, p)
	}
	return home, others
}

func write(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestRemoveEverything(t *testing.T) {
	home, others := fakeHome(t)
	removed, err := Remove(home, false)
	if err != nil {
		t.Fatal(err)
	}
	f := For(runtime.GOOS, home)
	want := slices.Clone(f.Dirs)
	if f.Autostart != "" {
		want = append(want, f.Autostart)
	}
	for _, p := range want {
		if exists(p) {
			t.Errorf("%s is still there", p)
		}
		if !slices.Contains(removed, p) {
			t.Errorf("%s isn't in removed %q", p, removed)
		}
	}
	for _, p := range others {
		if !exists(p) {
			t.Errorf("%s was removed; it isn't ours", p)
		}
	}
}

func TestRemoveAutostartOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows entry is a registry value")
	}
	home, _ := fakeHome(t)
	f := For(runtime.GOOS, home)
	removed, err := Remove(home, true)
	if err != nil {
		t.Fatal(err)
	}
	if exists(f.Autostart) || !slices.Equal(removed, []string{f.Autostart}) {
		t.Errorf("removed %q; autostart there: %v", removed, exists(f.Autostart))
	}
	for _, d := range f.Dirs {
		if !exists(d) {
			t.Errorf("%s was removed with autostartOnly", d)
		}
	}
}

func TestRemoveNothingThere(t *testing.T) {
	removed, err := Remove(t.TempDir(), false)
	if err != nil || len(removed) != 0 {
		t.Errorf("Remove on an empty home = %q, %v", removed, err)
	}
	removed, err = Remove(filepath.Join(t.TempDir(), "gone"), false)
	if err != nil || len(removed) != 0 {
		t.Errorf("Remove on a missing home = %q, %v", removed, err)
	}
}

// TestRemoveDoesNotFollowSymlinks covers a user who links one of the
// app's folders, or a folder above it, to somewhere outside their home.
// Run as root on someone else's home, following it would delete files
// the user couldn't.
func TestRemoveDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need a privilege on Windows")
	}
	f := relFor(runtime.GOOS)

	t.Run("the app's folder", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		write(t, filepath.Join(outside, "precious"))
		link := filepath.Join(home, f.Dirs[0])
		if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		if _, err := Remove(home, false); err != nil {
			t.Fatal(err)
		}
		if exists(link) {
			t.Error("the link is still there")
		}
		if !exists(filepath.Join(outside, "precious")) {
			t.Error("followed the link and removed what it points to")
		}
	})

	t.Run("a folder above it", func(t *testing.T) {
		home := t.TempDir()
		outside := t.TempDir()
		// The top folder of the app's first path (.config, Library).
		top := strings.SplitN(f.Dirs[0], "/", 2)
		target := filepath.Join(outside, top[1])
		write(t, filepath.Join(target, "precious"))
		if err := os.Symlink(outside, filepath.Join(home, top[0])); err != nil {
			t.Fatal(err)
		}
		_, err := Remove(home, false)
		if err == nil {
			t.Error("no error for a path that leaves the home folder")
		}
		if !exists(filepath.Join(target, "precious")) {
			t.Error("followed a link out of the home folder")
		}
	})
}
