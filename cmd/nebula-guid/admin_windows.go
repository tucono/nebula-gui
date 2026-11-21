// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import "golang.org/x/sys/windows"

// isAdmin reports whether this process runs elevated.
func isAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }
