// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// trayResource makes r a template: macOS draws menu bar icons in the bar's
// own colour, so the shapes, not the colours, tell the states apart.
func trayResource(r fyne.Resource) fyne.Resource { return theme.NewThemedResource(r) }
