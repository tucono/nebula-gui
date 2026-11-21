//go:build !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import "os"

// isAdmin reports whether this process runs as root.
func isAdmin() bool { return os.Geteuid() == 0 }
