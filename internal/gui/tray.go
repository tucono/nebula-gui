// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	appName = "nebula-gui"
	// trayReapply is how many renders reapply the tray even when nothing
	// changed. Windows refuses a tooltip until its tray icon exists, which
	// happens a moment after the menu is set.
	trayReapply = 3

	// Preferences that remember a notice was shown once.
	prefTrayNotice = "notice.closeToTray"
	prefQuitNotice = "notice.quitKeepsConnection"

	needsAttention = "Not connected · needs attention"
)

// trayView is what the tray shows. It is plain data so it can be tested
// without Fyne.
type trayView struct {
	Icon trayIcon
	// Status is the tooltip and the menu's first, disabled line; some
	// Linux trays don't show tooltips.
	Status string
	// Action is Connect or Disconnect, labelled Label.
	Action        action
	Label         string
	ActionEnabled bool
}

// trayModel turns what the window knows into what the tray shows.
func trayModel(in statusInput) trayView {
	v := trayView{Icon: trayOff, Status: "Not connected", Action: actConnect, Label: "Connect"}
	switch {
	case in.Err != nil || in.Version != nil:
		v.Icon, v.Status = trayAttention, needsAttention
		return v
	case in.Status == nil:
		return v // before the first answer
	case !in.Status.Configured:
		v.Status = "Not connected · no config"
		return v
	}

	st := in.Status
	switch st.State {
	case api.StateRunning:
		v = trayView{Icon: trayOn, Status: "Connected", Action: actDisconnect, Label: "Disconnect", ActionEnabled: true}
		if len(st.OverlayIPs) > 0 {
			ip, _, _ := strings.Cut(st.OverlayIPs[0], "/")
			v.Status += " · " + ip
		}
	case api.StateStarting:
		v.Icon, v.Status = trayBusy, "Connecting…"
	case api.StateStopping:
		v = trayView{Icon: trayBusy, Status: "Disconnecting…", Action: actDisconnect, Label: "Disconnect"}
	case api.StateError:
		v.Icon, v.Status, v.ActionEnabled = trayAttention, needsAttention, true
	default:
		v.ActionEnabled = true
		if in.Action != nil {
			v.Icon, v.Status = trayAttention, needsAttention
		}
	}

	switch in.Acting {
	case actNone:
	case actDisconnect:
		v.Icon, v.Status, v.ActionEnabled = trayBusy, "Disconnecting…", false
	case actRemoveConfig:
		v.Icon, v.Status, v.ActionEnabled = trayBusy, "Removing the config…", false
	default:
		v.Icon, v.Status, v.ActionEnabled = trayBusy, "Connecting…", false
	}
	return v
}

// trayHost is the desktop's tray. Tests replace it.
type trayHost interface {
	SetMenu(*fyne.Menu)
	SetIcon(fyne.Resource)
	SetTooltip(string)
	// SetOnTapped is called for a left click on the icon, on the UI
	// goroutine.
	SetOnTapped(func())
}

// attachTray puts the app in the tray: closing the window hides it there.
func (m *mainWindow) attachTray(t trayHost) {
	m.tray = t
	m.w.SetCloseIntercept(m.hideToTray)
	t.SetOnTapped(m.showWindow)
	m.renderTray()
}

// renderTray updates the tray from m.in when what it shows has changed.
func (m *mainWindow) renderTray() {
	if m.tray == nil {
		return
	}
	v := trayModel(m.now())
	if v == m.trayLast && m.trayApplied >= trayReapply {
		return
	}
	m.trayLast = v
	m.trayApplied++
	m.tray.SetMenu(m.trayMenu(v))
	m.tray.SetIcon(trayIcons[v.Icon])
	m.tray.SetTooltip(v.Status)
}

func (m *mainWindow) trayMenu(v trayView) *fyne.Menu {
	status := fyne.NewMenuItem(v.Status, nil)
	status.Disabled = true
	act := fyne.NewMenuItem(v.Label, func() { m.trayAct(v.Action) })
	act.Disabled = !v.ActionEnabled
	quit := fyne.NewMenuItem("Quit", m.quit)
	quit.IsQuit = true
	return fyne.NewMenu(appName,
		status,
		fyne.NewMenuItemSeparator(),
		act,
		fyne.NewMenuItem("Open "+appName, m.showWindow),
		fyne.NewMenuItemSeparator(),
		quit,
	)
}

// trayAct runs Connect or Disconnect from the tray menu. The menu may be a
// moment out of date, so the action is checked against the current state.
func (m *mainWindow) trayAct(a action) {
	v := trayModel(m.now())
	if !v.ActionEnabled || v.Action != a {
		return
	}
	m.actFrom(a, true)
}

// showWindow shows and raises the window, and polls at the normal rate
// again.
func (m *mainWindow) showWindow() {
	m.w.Show()
	m.w.RequestFocus()
	m.setHidden(false)
}

// hideToTray is what closing the window does while there's a tray. The
// first time, a notification says where the app went.
func (m *mainWindow) hideToTray() {
	m.w.Hide()
	m.setHidden(true)
	m.log.Add("window hidden in the tray")
	if p := m.a.Preferences(); !p.Bool(prefTrayNotice) {
		p.SetBool(prefTrayNotice, true)
		m.a.SendNotification(fyne.NewNotification(appName, appName+" is still running in the tray."))
	}
}

// setHidden switches the pollers between the shown and hidden window's
// rates.
func (m *mainWindow) setHidden(on bool) {
	m.hidden = on
	m.poller.SetHidden(on)
	m.logPoller.SetHidden(on)
}

// quit quits the app, from the tray or File → Quit. The connection stays
// up; the first time that matters, it says so and asks first.
func (m *mainWindow) quit() {
	p := m.a.Preferences()
	connected := m.in.Status != nil && m.in.Status.State == api.StateRunning
	if !connected || p.Bool(prefQuitNotice) {
		m.exit()
		return
	}
	p.SetBool(prefQuitNotice, true)
	m.showWindow()
	d := dialog.NewConfirm("Quit "+appName+"?", "The connection stays on. Disconnect first if you want it off.",
		func(ok bool) {
			if ok {
				m.exit()
			}
		}, m.w)
	d.SetConfirmText("Quit")
	d.SetDismissText("Cancel")
	d.Show()
}
