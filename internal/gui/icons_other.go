//go:build !darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import "fyne.io/fyne/v2"

// trayResource: other trays show the icon's own colours.
func trayResource(r fyne.Resource) fyne.Resource { return r }
