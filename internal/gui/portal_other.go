//go:build !linux && !darwin && !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

// platformOpen is nil where there's no native picker; Fyne's is used.
var platformOpen nativeOpenFunc
