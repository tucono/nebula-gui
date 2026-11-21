//go:build darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
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

// runUninstaller opens the uninstaller app, which can only be the one
// findUninstaller found. open returns once it has started.
func runUninstaller(app string) error {
	if app != darwinUninstaller {
		return fmt.Errorf("no uninstaller at %s", app)
	}
	return exec.Command("open", "-a", darwinUninstaller).Run()
}
