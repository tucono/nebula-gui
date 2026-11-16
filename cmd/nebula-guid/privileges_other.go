// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build !linux && !darwin && !windows

package main

// checkNetAdmin doesn't check anything on this OS; Nebula reports a
// missing permission when it starts.
func checkNetAdmin() error { return nil }
