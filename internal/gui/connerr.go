// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
)

// fromTransportError turns an error that never reached the service's error
// handling (the dial failed, the request timed out, the connection broke)
// into a catalogue entry. The errno lists live in connerr_unix.go and
// connerr_windows.go.
func fromTransportError(err error, goos string) *problem {
	switch {
	case isTimeout(err):
		return serviceNotResponding()
	case errnoIn(err, permissionErrnos):
		return noPermission(goos)
	case errnoIn(err, notRunningErrnos),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		// A connection that breaks mid-request means the service stopped.
		return serviceNotRunning()
	default:
		// Anything else (a garbled response, say) is still a service that
		// didn't give a usable answer, and Try again is the right advice.
		return serviceNotResponding()
	}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// http.Client.Timeout and connection deadlines.
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func errnoIn(err error, list []syscall.Errno) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	for _, e := range list {
		if errno == e {
			return true
		}
	}
	return false
}
