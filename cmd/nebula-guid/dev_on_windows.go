// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build dev

package main

import "os/user"

// devSocketOwner returns the current user. The dev service runs elevated,
// and the GUI runs as the same user without the Administrators group in its
// token, so the user needs its own entry in the pipe's DACL.
func devSocketOwner() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
