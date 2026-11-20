//go:build !darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import "errors"

// findUninstaller: only macOS has an uninstaller app. Windows has
// Settings → Apps, and Linux the package manager.
func findUninstaller() string { return "" }

func runUninstaller(string) error {
	return errors.New("there's no uninstaller app on this OS")
}
