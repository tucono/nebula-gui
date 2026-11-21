// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isAddrInUse reports a Winsock "address in use" error. syscall.EADDRINUSE
// is an invented value on Windows and never matches.
func isAddrInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }
