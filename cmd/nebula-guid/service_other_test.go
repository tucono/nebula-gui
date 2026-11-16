// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build !windows

package main

import (
	"errors"
	"testing"
)

func TestWindowsCommandsRefused(t *testing.T) {
	for _, cmd := range []string{"install", "uninstall", "run"} {
		if err := run([]string{cmd}); !errors.Is(err, errWindowsOnly) {
			t.Errorf("%s: err = %v", cmd, err)
		}
	}
}
