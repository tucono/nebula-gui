// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"os"
)

// checkNetAdmin fails unless the process runs as root, which macOS needs
// to create a utun interface.
func checkNetAdmin() error {
	if os.Geteuid() == 0 {
		return nil
	}
	return errors.New("must run as root to create the Nebula network interface. " +
		"Install the package, which runs it as a launch daemon, or run it with sudo")
}
