// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build dev && unix

package main

import "os"

// devSocketOwner returns the user who ran sudo, so the GUI can connect when
// the dev service runs as root. Without sudo the socket already belongs to
// the current user.
func devSocketOwner() string { return os.Getenv("SUDO_USER") }
