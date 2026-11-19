//go:build !linux && !darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

// platformOpen is nil until there's a native picker for this OS (Windows
// comes with its installer); Fyne's picker is used meanwhile.
var platformOpen nativeOpenFunc
