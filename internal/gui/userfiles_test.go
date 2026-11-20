// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"fyne.io/fyne/v2/app"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

// TestFynePreferencesFolder checks that Fyne keeps the app's preferences
// where userfiles says, so an uninstaller removes them. A Fyne upgrade
// that moves them fails here.
func TestFynePreferencesFolder(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "") // the default, as an uninstaller sees it
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder:", err)
	}
	root := app.NewWithID(appID).Storage().RootURI()
	if root == nil {
		t.Fatal("Fyne gave no storage folder")
	}
	got := filepath.FromSlash(root.Path())
	if want := userfiles.For(runtime.GOOS, home).Dirs; !slices.Contains(want, got) {
		t.Errorf("Fyne's storage folder is %q, not in %q", got, want)
	}
}
