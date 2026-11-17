// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"path/filepath"
	"testing"
)

func TestIsTempBuild(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		exe  string
		want bool
	}{
		{filepath.Join(tmp, "go-build123456", "b001", "exe", "nebula-gui"), true},
		{filepath.Join(tmp, "go-build789", "b001", "nebula-gui.test"), true},
		{filepath.Join(tmp, "bin", "nebula-gui"), false},
		{filepath.Join(tmp, "my-go-build", "nebula-gui"), false},
	}
	for _, c := range cases {
		if got := IsTempBuild(c.exe); got != c.want {
			t.Errorf("IsTempBuild(%q) = %v, want %v", c.exe, got, c.want)
		}
	}
}

func TestNewNeedsAnAbsolutePath(t *testing.T) {
	if _, err := New(filepath.Join("bin", "nebula-gui")); err == nil {
		t.Fatal("a relative path was accepted")
	}
}
