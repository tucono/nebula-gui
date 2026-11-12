// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.Load(); ok || err != nil {
		t.Fatalf("Load on empty store: ok=%v err=%v", ok, err)
	}
	if err := s.Save("a: 1\n"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("a: 2\n"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.Load()
	if err != nil || !ok || got != "a: 2\n" {
		t.Fatalf("Load = %q, %v, %v", got, ok, err)
	}

	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, configFile): 0o600} {
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != want {
				t.Errorf("%s mode = %v, want %v", path, fi.Mode().Perm(), want)
			}
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover temp files: %v", entries)
	}

	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("second Delete: %v", err)
	}
	if _, ok, _ := s.Load(); ok {
		t.Fatal("config still present after Delete")
	}
}

func TestOpenStoreTightensExistingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions are a DACL on Windows")
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(dir); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v", fi.Mode().Perm())
	}
}
