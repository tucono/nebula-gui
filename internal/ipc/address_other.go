// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix && !linux && !darwin

package ipc

// DefaultAddress is where the installed service listens.
func DefaultAddress() string { return "/var/run/nebula-guid.sock" }
