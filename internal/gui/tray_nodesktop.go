//go:build android || ios || js || wasm

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import "fyne.io/fyne/v2"

// newDesktopTray: there's no tray off the desktop.
func newDesktopTray(fyne.App) trayHost { return nil }
