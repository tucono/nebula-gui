//go:build darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"os"
	"os/exec"
)

// darwinUninstaller is the app the package installs beside nebula-gui,
// built from packaging/macos/uninstall.applescript.
const darwinUninstaller = "/Applications/Uninstall nebula-gui.app"

// findUninstaller returns the uninstaller app, or "" if it isn't there.
func findUninstaller() string {
	if _, err := os.Stat(darwinUninstaller); err != nil {
		return ""
	}
	return darwinUninstaller
}

// runUninstaller opens the uninstaller app. open returns once it has
// started.
func runUninstaller(app string) error {
	return exec.Command("open", "-a", app).Run()
}
