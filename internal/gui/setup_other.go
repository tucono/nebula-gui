//go:build !linux && !darwin && !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
)

// probeSetup knows nothing here: there's no package for this OS.
func probeSetup() serviceSetup { return serviceSetup{} }

func startService(context.Context) error {
	return errors.New("starting the service isn't supported on this OS")
}
