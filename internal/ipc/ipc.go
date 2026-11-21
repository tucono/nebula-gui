// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package ipc provides the local transport between the GUI and the service:
// a Unix socket on Linux and macOS, a named pipe on Windows. There is never a
// TCP listener.
package ipc

import "errors"

// ErrInUse is returned by Listen when another service is already accepting
// connections at the address.
var ErrInUse = errors.New("address already in use by a running service")

// ListenOptions controls who may connect to the listener. With neither field
// set, only the service's own user (and, on Windows, SYSTEM and
// Administrators) can connect.
type ListenOptions struct {
	// Owner is a user name that is given access. On Unix it becomes the
	// socket's owner, which needs root unless it is the current user.
	Owner string
	// Group is a group name whose members are given access. On Unix the
	// socket's group is set to it and the mode becomes 0660.
	Group string
}
