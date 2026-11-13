//go:build !linux

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

// platformOpen is nil until there's a native picker for this OS (Windows
// and macOS come with the M4 installers); Fyne's picker is used meanwhile.
var platformOpen nativeOpenFunc
