//go:build windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	// notRunningErrnos: no pipe with that name, or the service went away
	// mid-request. A busy pipe is retried by go-winio until the deadline,
	// so it shows up as a timeout instead.
	notRunningErrnos = []syscall.Errno{
		windows.ERROR_FILE_NOT_FOUND, windows.ERROR_PATH_NOT_FOUND,
		windows.ERROR_BROKEN_PIPE, windows.ERROR_PIPE_NOT_CONNECTED, windows.ERROR_NO_DATA,
	}
	// permissionErrnos: the pipe's DACL keeps this user out.
	permissionErrnos = []syscall.Errno{windows.ERROR_ACCESS_DENIED}
)
