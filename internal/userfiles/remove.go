// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// Remove removes a user's nebula-gui files under home, or only the
// start-at-sign-in file with autostartOnly. It returns the paths it
// removed. A file that isn't there isn't an error; each one that can't be
// removed is, and the rest are still tried.
//
// Uninstallers run it as root or administrator on other users' home
// folders, so every path is opened through an os.Root at home: a symlink
// a user placed in their home folder can't lead a removal outside it.
// A symlink to elsewhere, at ~/.config for example, makes those files
// fail to remove rather than following it.
func Remove(home string, autostartOnly bool) (removed []string, err error) {
	root, err := os.OpenRoot(home)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // no home folder, so no files
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	f := relFor(runtime.GOOS)
	var names []string
	if f.Autostart != "" {
		names = append(names, f.Autostart)
	}
	if !autostartOnly {
		names = append(names, f.Dirs...)
	}
	var errs []error
	for _, name := range names {
		if _, err := root.Lstat(name); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		// RemoveAll removes a symlink itself, never what it points to.
		if err := root.RemoveAll(name); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", filepath.Join(home, name), err))
			continue
		}
		removed = append(removed, filepath.Join(home, name))
	}
	return removed, errors.Join(errs...)
}
