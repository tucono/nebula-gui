//go:build darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"os"
)

// darwinServicePlist is where the package installs the service's job.
const darwinServicePlist = "/Library/LaunchDaemons/" + darwinServiceLabel + ".plist"

// probeSetup checks for the job. The app doesn't start it: the problem
// gives the command instead.
func probeSetup() serviceSetup {
	_, err := os.Stat(darwinServicePlist)
	return serviceSetup{Known: true, Installed: err == nil}
}

func startService(context.Context) error {
	return errors.New("starting the service isn't supported on macOS")
}
