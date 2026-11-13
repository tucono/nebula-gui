//go:build unix

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import "syscall"

var (
	// notRunningErrnos: no socket file, nothing listening on it, or the
	// service went away mid-request.
	notRunningErrnos = []syscall.Errno{syscall.ENOENT, syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EPIPE}
	// permissionErrnos: the socket's mode or group keeps this user out.
	permissionErrnos = []syscall.Errno{syscall.EACCES, syscall.EPERM}
)
