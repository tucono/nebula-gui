//go:build !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl

import (
	"errors"
	"syscall"
)

func isAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }
