//go:build !linux && !darwin && !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

// osVersion is unknown on this platform.
func osVersion() string { return "" }
