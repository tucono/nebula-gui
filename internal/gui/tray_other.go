//go:build !linux || android

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

// hasTray: Windows and macOS always have a notification area or menu bar.
func hasTray() bool { return true }
