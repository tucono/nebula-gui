// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"embed"

	"fyne.io/fyne/v2"
)

// The icons are placeholders: replace the SVGs in assets/ and run
// make icons to regenerate the packages' PNG, ICO and ICNS files.
//
//go:embed assets/*.svg
var assets embed.FS

// appIcon is the window and notification icon.
var appIcon = asset("app.svg")

// trayIcon is one of the tray's looks.
type trayIcon int

const (
	trayOff trayIcon = iota
	trayOn
	trayBusy
	trayAttention
)

func (i trayIcon) String() string {
	switch i {
	case trayOn:
		return "on"
	case trayBusy:
		return "busy"
	case trayAttention:
		return "attention"
	default:
		return "off"
	}
}

var trayIcons = map[trayIcon]fyne.Resource{
	trayOff:       trayAsset("tray-off.svg"),
	trayOn:        trayAsset("tray-on.svg"),
	trayBusy:      trayAsset("tray-busy.svg"),
	trayAttention: trayAsset("tray-attention.svg"),
}

func asset(name string) fyne.Resource {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		panic(err) // embedded at build time; can't be missing
	}
	return fyne.NewStaticResource(name, b)
}

// trayAsset is a tray icon, as the platform wants it (see trayResource).
func trayAsset(name string) fyne.Resource { return trayResource(asset(name)) }
