// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package autostart starts the app hidden in the tray when the user signs
// in: an XDG autostart file on Linux, a LaunchAgent on macOS, and a Run
// value in the user's registry on Windows. Each is per user and needs no
// administrator rights.
package autostart

import (
	"errors"
	"path/filepath"
	"strings"
)

// HiddenFlag is passed to the app at sign-in, so it starts in the tray.
const HiddenFlag = "--hidden"

// ErrUnsupported is returned by New where the app can't start at sign-in.
var ErrUnsupported = errors.New("starting at sign-in isn't supported here")

// Entry is this user's start-at-sign-in entry for the app.
type Entry interface {
	// Enabled reports whether the app starts at sign-in. It reads the
	// file or registry value each time, so removing it by hand counts.
	Enabled() (bool, error)
	// SetEnabled creates or removes the entry.
	SetEnabled(on bool) error
}

// New returns the entry that starts exe at sign-in.
func New(exe string) (Entry, error) {
	if !filepath.IsAbs(exe) {
		return nil, errors.New("the app's path isn't absolute: " + exe)
	}
	return newEntry(exe)
}

// IsTempBuild reports whether exe was built by go run or go test into a
// temporary directory, which is gone by the next sign-in.
func IsTempBuild(exe string) bool {
	for d := filepath.Dir(exe); ; {
		if strings.HasPrefix(filepath.Base(d), "go-build") {
			return true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return false
		}
		d = parent
	}
}
