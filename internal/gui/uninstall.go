// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// uninstallItem is Help → Uninstall nebula-gui…, which opens the
// uninstaller app the macOS package installs. It's nil elsewhere, and in
// a development build, where there's no uninstaller.
func (m *mainWindow) uninstallItem() *fyne.MenuItem {
	if m.uninstaller == "" {
		return nil
	}
	return fyne.NewMenuItem("Uninstall "+appName+"…", m.openUninstaller)
}

// openUninstaller opens the uninstaller app and quits, so the app isn't
// running while it's removed. The uninstaller asks before removing
// anything; cancelling it leaves the app installed, to be opened again.
func (m *mainWindow) openUninstaller() {
	m.log.Add("uninstaller opened")
	if err := m.runUninstaller(m.uninstaller); err != nil {
		m.log.Add("uninstaller open failed", "error", err.Error())
		dialog.ShowError(err, m.w)
		return
	}
	m.exit()
}
