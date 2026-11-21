//go:build !android && !ios && !js && !wasm

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/systray"
)

// desktopTray is the tray through Fyne, plus the two things Fyne doesn't
// expose: the tooltip, and a left click that goes through showWindow so
// the pollers know the window is back.
type desktopTray struct {
	app desktop.App
}

// newDesktopTray returns the tray, or nil when there isn't one: on Linux
// that's a desktop without a StatusNotifier host, such as GNOME without
// the AppIndicator extension. It's checked once, at start-up.
func newDesktopTray(a fyne.App) trayHost {
	d, ok := a.(desktop.App)
	if !ok || !hasTray() {
		return nil
	}
	return &desktopTray{app: d}
}

func (t *desktopTray) SetMenu(m *fyne.Menu)    { t.app.SetSystemTrayMenu(m) }
func (t *desktopTray) SetIcon(r fyne.Resource) { t.app.SetSystemTrayIcon(r) }
func (t *desktopTray) SetTooltip(s string)     { systray.SetTooltip(s) }
func (t *desktopTray) SetOnTapped(f func())    { systray.SetOnTapped(func() { fyne.Do(f) }) }
func (t *desktopTray) Available() bool         { return hasTray() }
