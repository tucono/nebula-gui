//go:build !darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

// watchReopen: elsewhere, opening the app again starts a second copy,
// which asks this one to show its window (see instance.go).
func watchReopen(func()) {}
