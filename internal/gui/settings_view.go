// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/autostart"
)

const (
	// prefAutostartDefault is set once start at sign-in has had its
	// default applied, so the app does it only on its first run.
	prefAutostartDefault = "autostart.defaultApplied"
	// prefAutostartNotice is set while the Settings tab still has to say
	// that the app turned start at sign-in on.
	prefAutostartNotice = "notice.autostartTurnedOn"
)

// settingsInput is what the Settings tab is drawn from.
type settingsInput struct {
	// Reachable is set while the service answers in this API version.
	Reachable bool
	// Service is the service's settings, nil until read.
	Service *api.Settings
	// Reading is set while they're being read.
	Reading bool
	// Saving is the autoconnect value being saved, or nil.
	Saving *bool
	// ServiceErr says why the last read or change failed.
	ServiceErr string

	// Tray is set when there's a tray: starting at sign-in hides the app
	// there, so it's offered only then.
	Tray bool
	// Autostart is set where the app can start at sign-in.
	Autostart bool
	// AutostartOn is whether it does.
	AutostartOn bool
	// AutostartErr says why the last read or change failed.
	AutostartErr string
	// TempBuild is set when the app runs from a temporary build.
	TempBuild bool
	// TurnedOn is set when the app turned start at sign-in on by itself,
	// on its first run.
	TurnedOn bool
}

// settingsModel is what the Settings tab shows.
type settingsModel struct {
	AutoconnectOn      bool
	AutoconnectEnabled bool
	AutoconnectNote    string

	ShowAutostart    bool
	AutostartOn      bool
	AutostartEnabled bool
	AutostartNote    string
}

const (
	noteUnreachable = "The background service isn't answering, so this can't be changed now."
	noteChecking    = "Checking…"
	noteTempBuild   = "This copy was built by go run, so it can't be started at sign-in. Build it with make gui-bin."
	noteTurnedOn    = "nebula-gui turned this on when it first ran. Untick it if you'd rather open it yourself."
)

func describeSettings(in settingsInput) settingsModel {
	var m settingsModel
	switch {
	case in.Saving != nil:
		m.AutoconnectOn = *in.Saving
	case in.Service != nil:
		m.AutoconnectOn = in.Service.Autoconnect
	}
	m.AutoconnectEnabled = in.Reachable && in.Service != nil && !in.Reading && in.Saving == nil
	switch {
	case !in.Reachable:
		m.AutoconnectNote = noteUnreachable
	case in.ServiceErr != "":
		m.AutoconnectNote = in.ServiceErr
	case in.Service == nil:
		m.AutoconnectNote = noteChecking
	}

	m.ShowAutostart = in.Tray && in.Autostart
	if !m.ShowAutostart {
		return m
	}
	m.AutostartOn = in.AutostartOn
	m.AutostartEnabled = !in.TempBuild
	switch {
	case in.AutostartErr != "":
		m.AutostartNote = in.AutostartErr
	case in.TempBuild:
		m.AutostartNote = noteTempBuild
	case in.TurnedOn && in.AutostartOn:
		m.AutostartNote = noteTurnedOn
	}
	return m
}

// settingsView is the Settings tab.
type settingsView struct {
	root            fyne.CanvasObject
	autoconnect     *widget.Check
	autoconnectNote *widget.Label
	autostartBox    *fyne.Container
	autostart       *widget.Check
	autostartNote   *widget.Label
	// applying is set while set changes the checks, so OnChanged knows
	// the user didn't.
	applying bool
}

func newSettingsView(onAutoconnect, onAutostart func(bool)) *settingsView {
	v := &settingsView{}
	v.autoconnect = widget.NewCheck("Connect automatically when this computer starts", func(on bool) {
		if !v.applying {
			onAutoconnect(on)
		}
	})
	v.autostart = widget.NewCheck("Open nebula-gui when I sign in", func(on bool) {
		if !v.applying {
			onAutostart(on)
		}
	})
	v.autoconnectNote = note("")
	v.autostartNote = note("")
	v.autostartBox = container.NewVBox(
		widget.NewSeparator(),
		v.autostart,
		note("It opens in the tray, without this window."),
		v.autostartNote,
	)
	// The scroll keeps the content from growing the window; see
	// newMainWindow.
	v.root = container.NewVScroll(container.NewPadded(container.NewVBox(
		v.autoconnect,
		note("This applies to everyone who uses this computer, and only when a config is imported."),
		v.autoconnectNote,
		v.autostartBox,
	)))
	return v
}

// note is a line of small print under a setting.
func note(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.SizeName = theme.SizeNameCaptionText
	return l
}

func (v *settingsView) set(m settingsModel) {
	v.applying = true
	defer func() { v.applying = false }()
	v.autoconnect.SetChecked(m.AutoconnectOn)
	setCheckEnabled(v.autoconnect, m.AutoconnectEnabled)
	setNote(v.autoconnectNote, m.AutoconnectNote)

	v.autostartBox.Hidden = !m.ShowAutostart
	v.autostart.SetChecked(m.AutostartOn)
	setCheckEnabled(v.autostart, m.AutostartEnabled)
	setNote(v.autostartNote, m.AutostartNote)
	v.autostartBox.Refresh()
}

func setCheckEnabled(c *widget.Check, on bool) {
	if on {
		c.Enable()
	} else {
		c.Disable()
	}
}

func setNote(l *widget.Label, s string) {
	l.SetText(s)
	l.Hidden = s == ""
	l.Refresh()
}

// settingsShowing reports whether the Settings tab is the selected one.
func (m *mainWindow) settingsShowing() bool {
	return m.tabs.Selected() != nil && m.tabs.Selected().Content == m.settings.root
}

// reachable reports whether the service answers in this API version.
func (m *mainWindow) reachable() bool {
	return m.in.Err == nil && m.in.Version == nil && m.in.Status != nil
}

// renderSettings updates the Settings tab. While the service can't be
// reached its settings are forgotten, so they're read again when it's
// back: it may have restarted, or another user changed them.
func (m *mainWindow) renderSettings() {
	in := &m.sets
	in.Reachable = m.reachable()
	if !in.Reachable && !in.Reading && in.Saving == nil {
		in.Service, in.ServiceErr = nil, ""
	}
	in.Tray = m.tray != nil
	in.Autostart = m.autostart != nil
	m.settings.set(describeSettings(*in))
}

// openSettings reads the settings again when the tab is selected, so a
// change made elsewhere shows.
func (m *mainWindow) openSettings() {
	m.sets.ServiceErr, m.sets.AutostartErr = "", ""
	m.readAutostart()
	if m.sets.TurnedOn {
		// Said once: this time, but not when the app next starts.
		m.a.Preferences().SetBool(prefAutostartNotice, false)
	}
	m.readSettings()
	m.render()
}

// maybeReadSettings reads the service's settings if the tab shows and
// they haven't been read since the service was last reachable.
func (m *mainWindow) maybeReadSettings() {
	if m.settingsShowing() && m.sets.Service == nil && m.sets.ServiceErr == "" {
		m.readSettings()
	}
}

// readSettings asks the service for its settings, off the UI goroutine.
func (m *mainWindow) readSettings() {
	if !m.reachable() || m.sets.Reading || m.sets.Saving != nil {
		return
	}
	m.sets.Reading = true
	m.renderSettings()
	m.spawn(func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		s, err := m.c.Settings(ctx)
		fyne.Do(func() {
			m.sets.Reading = false
			if err != nil {
				m.sets.ServiceErr = "Couldn't read this setting from the background service."
				m.log.Add("couldn't read the service's settings", "error", errorDetails(err))
			} else {
				m.sets.Service, m.sets.ServiceErr = s, ""
			}
			m.renderSettings()
		})
	})
}

// setAutoconnect saves the service's autoconnect setting. The check shows
// the new value while it's saved, and the stored one afterwards.
func (m *mainWindow) setAutoconnect(on bool) {
	if !describeSettings(m.sets).AutoconnectEnabled {
		m.renderSettings() // undo the click
		return
	}
	m.sets.Saving, m.sets.ServiceErr = &on, ""
	m.renderSettings()
	m.spawn(func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		s, err := m.c.PutSettings(ctx, api.Settings{Autoconnect: on})
		fyne.Do(func() {
			m.sets.Saving = nil
			if err != nil {
				m.sets.ServiceErr = "Couldn't change this setting. Try again in a moment."
				m.log.Add("couldn't change autoconnect", "on", onOff(on), "error", errorDetails(err))
			} else {
				m.sets.Service = s
				m.log.Add("autoconnect changed", "on", onOff(s.Autoconnect))
			}
			m.renderSettings()
		})
	})
}

// setAutostartEntry is the platform's start-at-sign-in entry, nil where
// there isn't one. tempBuild is set when the app runs from a temporary
// build, which can't be started later.
func (m *mainWindow) setAutostartEntry(e autostart.Entry, tempBuild bool) {
	m.autostart = e
	m.sets.TempBuild = tempBuild
	m.sets.TurnedOn = m.a.Preferences().Bool(prefAutostartNotice)
	m.readAutostart()
	m.renderSettings()
}

func (m *mainWindow) readAutostart() {
	if m.autostart == nil {
		return
	}
	on, err := m.autostart.Enabled()
	if err != nil {
		m.sets.AutostartErr = "Couldn't check whether nebula-gui opens when you sign in."
		m.log.Add("couldn't read start at sign-in", "error", err.Error())
		return
	}
	m.sets.AutostartOn = on
}

// setAutostart turns start at sign-in on or off.
func (m *mainWindow) setAutostart(on bool) {
	if m.autostart == nil || m.sets.TempBuild {
		m.renderSettings()
		return
	}
	m.sets.AutostartErr, m.sets.TurnedOn = "", false
	m.a.Preferences().SetBool(prefAutostartNotice, false)
	if err := m.autostart.SetEnabled(on); err != nil {
		m.sets.AutostartErr = "Couldn't change this setting: " + err.Error()
		m.log.Add("couldn't change start at sign-in", "on", onOff(on), "error", err.Error())
	} else {
		m.log.Add("start at sign-in changed", "on", onOff(on))
	}
	m.readAutostart()
	m.renderSettings()
}

// applyAutostartDefault turns start at sign-in on the first time the app
// runs with a tray, unless it already is. It's done once, so turning it
// off sticks. The Settings tab says so the next time it's opened.
func (m *mainWindow) applyAutostartDefault() {
	if m.tray == nil || m.autostart == nil || m.sets.TempBuild {
		return
	}
	p := m.a.Preferences()
	if p.Bool(prefAutostartDefault) {
		return
	}
	p.SetBool(prefAutostartDefault, true)
	if on, err := m.autostart.Enabled(); err != nil || on {
		return
	}
	if err := m.autostart.SetEnabled(true); err != nil {
		m.log.Add("couldn't turn on start at sign-in", "error", err.Error())
		return
	}
	p.SetBool(prefAutostartNotice, true)
	m.sets.TurnedOn = true
	m.log.Add("start at sign-in turned on, the default")
	m.readAutostart()
	m.renderSettings()
}
