// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
)

func TestFor(t *testing.T) {
	tests := []struct {
		goos, home string
		want       Files
	}{
		{"linux", "/home/alice", Files{
			Dirs: []string{
				"/home/alice/.config/nebula-gui",
				"/home/alice/.config/fyne/io.github.tucono.nebulagui",
				"/home/alice/.cache/nebula-gui",
			},
			Autostart: "/home/alice/.config/autostart/nebula-gui.desktop",
		}},
		{"darwin", "/Users/alice", Files{
			Dirs: []string{
				"/Users/alice/Library/Application Support/nebula-gui",
				"/Users/alice/Library/Preferences/fyne/io.github.tucono.nebulagui",
				"/Users/alice/Library/Caches/nebula-gui",
			},
			Autostart: "/Users/alice/Library/LaunchAgents/io.github.tucono.nebulagui.plist",
		}},
		{"windows", `C:\Users\alice`, Files{
			Dirs: []string{
				`C:\Users\alice\AppData\Roaming\nebula-gui`,
				`C:\Users\alice\AppData\Roaming\fyne\io.github.tucono.nebulagui`,
			},
		}},
	}
	for _, tt := range tests {
		if got := For(tt.goos, tt.home); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("For(%q, %q) =\n%+v\nwant\n%+v", tt.goos, tt.home, got, tt.want)
		}
	}
}

// TestForMatchesThisUser checks For against where the standard library
// puts this user's config and cache folders, so the uninstaller looks
// where the app writes. Fyne's folder is checked in internal/gui.
func TestForMatchesThisUser(t *testing.T) {
	// The defaults, as an uninstaller sees them from outside the session.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder:", err)
	}
	f := For(runtime.GOOS, home)

	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Dirs, dir) {
		t.Errorf("Dir() = %q, not in %q", dir, f.Dirs)
	}
	if runtime.GOOS == "windows" {
		return // the app's cache folder holds nothing on Windows
	}
	cache, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.Dirs, cache) {
		t.Errorf("CacheDir() = %q, not in %q", cache, f.Dirs)
	}
	if runtime.GOOS == "linux" {
		cfg, _ := os.UserConfigDir()
		if want := filepath.Join(cfg, "autostart", AutostartName); f.Autostart != want {
			t.Errorf("Autostart = %q, want %q", f.Autostart, want)
		}
	}
}
