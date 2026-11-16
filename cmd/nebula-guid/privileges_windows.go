// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

// checkNetAdmin fails unless the process is elevated, which Wintun needs
// to create the adapter. The service runs as LocalSystem, which is.
func checkNetAdmin() error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return nil
	}
	return errors.New("must run as an administrator to create the Nebula network adapter. " +
		`Install it as a service with "nebula-guid install", or run it from an elevated prompt`)
}
