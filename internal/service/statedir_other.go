//go:build !linux && !darwin && !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

// DefaultStateDir is where the installed service keeps its config.
func DefaultStateDir() string { return "/var/lib/nebula-gui" }
