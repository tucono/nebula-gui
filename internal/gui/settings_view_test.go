// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"slices"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

func (f *fakeService) Settings(context.Context) (*api.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "settings")
	switch {
	case f.down:
		return nil, errRefused
	case f.settingsErr != nil:
		return nil, f.settingsErr
	}
	return &api.Settings{Autoconnect: f.autoconnect}, nil
}

func (f *fakeService) PutSettings(_ context.Context, s api.Settings) (*api.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "put settings "+onOff(s.Autoconnect))
	switch {
	case f.down:
		return nil, errRefused
	case f.putSettingsErr != nil:
		return nil, f.putSettingsErr
	}
	f.autoconnect = s.Autoconnect
	return &api.Settings{Autoconnect: f.autoconnect}, nil
}

// fakeAutostart is a start-at-sign-in entry in memory.
type fakeAutostart struct {
	on     bool
	err    error // returned by SetEnabled when set
	readEr error // returned by Enabled when set
	sets   []bool
}

func (f *fakeAutostart) Enabled() (bool, error) { return f.on, f.readEr }

func (f *fakeAutostart) SetEnabled(on bool) error {
	f.sets = append(f.sets, on)
	if f.err != nil {
		return f.err
	}
	f.on = on
	return nil
}

func TestDescribeSettings(t *testing.T) {
	on, off := true, false
	svcOn := &api.Settings{Autoconnect: true}
	svcOff := &api.Settings{}
	cases := []struct {
		name string
		in   settingsInput
		want settingsModel
	}{
		{"service unreachable", settingsInput{},
			settingsModel{AutoconnectNote: noteUnreachable}},
		{"reading", settingsInput{Reachable: true, Reading: true},
			settingsModel{AutoconnectNote: noteChecking}},
		{"read: off", settingsInput{Reachable: true, Service: svcOff},
			settingsModel{AutoconnectEnabled: true}},
		{"read: on", settingsInput{Reachable: true, Service: svcOn},
			settingsModel{AutoconnectOn: true, AutoconnectEnabled: true}},
		{"saving on", settingsInput{Reachable: true, Service: svcOff, Saving: &on},
			settingsModel{AutoconnectOn: true}},
		{"saving off", settingsInput{Reachable: true, Service: svcOn, Saving: &off},
			settingsModel{}},
		{"couldn't read", settingsInput{Reachable: true, ServiceErr: "no"},
			settingsModel{AutoconnectNote: "no"}},
		{"couldn't save", settingsInput{Reachable: true, Service: svcOn, ServiceErr: "no"},
			settingsModel{AutoconnectOn: true, AutoconnectEnabled: true, AutoconnectNote: "no"}},
		{"unreachable beats an error", settingsInput{ServiceErr: "no"},
			settingsModel{AutoconnectNote: noteUnreachable}},

		{"sign-in without a tray", settingsInput{Autostart: true, AutostartOn: true},
			settingsModel{AutoconnectNote: noteUnreachable}},
		{"sign-in unsupported", settingsInput{Tray: true},
			settingsModel{AutoconnectNote: noteUnreachable}},
		{"sign-in off", settingsInput{Tray: true, Autostart: true},
			settingsModel{AutoconnectNote: noteUnreachable, ShowAutostart: true, AutostartEnabled: true}},
		{"sign-in on", settingsInput{Tray: true, Autostart: true, AutostartOn: true},
			settingsModel{AutoconnectNote: noteUnreachable, ShowAutostart: true, AutostartOn: true, AutostartEnabled: true}},
		{"sign-in turned on by the app", settingsInput{Tray: true, Autostart: true, AutostartOn: true, TurnedOn: true},
			settingsModel{AutoconnectNote: noteUnreachable, ShowAutostart: true, AutostartOn: true, AutostartEnabled: true,
				AutostartNote: noteTurnedOn}},
		{"sign-in turned on by the app, then off by hand", settingsInput{Tray: true, Autostart: true, TurnedOn: true},
			settingsModel{AutoconnectNote: noteUnreachable, ShowAutostart: true, AutostartEnabled: true}},
		{"sign-in from a temporary build", settingsInput{Tray: true, Autostart: true, TempBuild: true},
			settingsModel{AutoconnectNote: noteUnreachable, ShowAutostart: true, AutostartNote: noteTempBuild}},
		{"sign-in error", settingsInput{Tray: true, Autostart: true, AutostartErr: "no", TurnedOn: true, AutostartOn: true},
			settingsModel{AutoconnectNote: noteUnreachable, ShowAutostart: true, AutostartOn: true, AutostartEnabled: true,
				AutostartNote: "no"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := describeSettings(c.in); got != c.want {
				t.Fatalf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// selectSettings opens the Settings tab.
func selectSettings(t *testing.T, m *mainWindow) {
	t.Helper()
	m.tabs.SelectIndex(3)
	if !m.settingsShowing() {
		t.Fatal("the Settings tab isn't the fourth")
	}
}

func TestSettingsAutoconnect(t *testing.T) {
	svc := newFakeService()
	m, poll := testWindow(t, svc)
	v := m.settings
	poll()
	if slices.Contains(svc.takeCalls(), "settings") {
		t.Fatal("settings read before the tab was opened")
	}

	selectSettings(t, m)
	if c := svc.takeCalls(); !slices.Equal(c, []string{"settings"}) {
		t.Fatalf("calls %v", c)
	}
	if v.autoconnect.Checked || v.autoconnect.Disabled() || v.autoconnectNote.Visible() {
		t.Fatalf("after reading: checked %v, disabled %v, note %q",
			v.autoconnect.Checked, v.autoconnect.Disabled(), v.autoconnectNote.Text)
	}

	test.Tap(v.autoconnect)
	if c := svc.takeCalls(); !slices.Equal(c, []string{"put settings on"}) {
		t.Fatalf("calls %v", c)
	}
	if !v.autoconnect.Checked || !svc.autoconnect {
		t.Fatal("not on after the tap")
	}
	if !slices.Contains(msgs(m.log), "autoconnect changed") {
		t.Fatalf("log %v", msgs(m.log))
	}

	// Polling doesn't read again; opening the tab again does.
	poll()
	if slices.Contains(svc.takeCalls(), "settings") {
		t.Fatal("settings read again by a poll")
	}
	svc.set(func(f *fakeService) { f.autoconnect = false }) // another user
	m.tabs.SelectIndex(0)
	selectSettings(t, m)
	if v.autoconnect.Checked {
		t.Fatal("a change made elsewhere didn't show")
	}

	// A failed change goes back to the stored value and says so.
	svc.set(func(f *fakeService) { f.putSettingsErr = &api.Error{Code: api.CodeInternal, Message: "disk full"} })
	test.Tap(v.autoconnect)
	if v.autoconnect.Checked || v.autoconnect.Disabled() || !v.autoconnectNote.Visible() {
		t.Fatalf("after a failed change: checked %v, disabled %v, note %q",
			v.autoconnect.Checked, v.autoconnect.Disabled(), v.autoconnectNote.Text)
	}
	if !slices.Contains(msgs(m.log), "couldn't change autoconnect") {
		t.Fatalf("log %v", msgs(m.log))
	}
}

func TestSettingsAutoconnectServiceDown(t *testing.T) {
	svc := newFakeService()
	svc.down = true
	m, poll := testWindow(t, svc)
	v := m.settings
	poll()
	selectSettings(t, m)
	if slices.Contains(svc.takeCalls(), "settings") {
		t.Fatal("tried to read the settings from an unreachable service")
	}
	if !v.autoconnect.Disabled() || v.autoconnectNote.Text != noteUnreachable {
		t.Fatalf("disabled %v, note %q", v.autoconnect.Disabled(), v.autoconnectNote.Text)
	}
	test.Tap(v.autoconnect)
	if v.autoconnect.Checked || slices.Contains(svc.takeCalls(), "put settings on") {
		t.Fatal("a disabled check was changed")
	}

	// Back up: read without the tab being opened again.
	svc.set(func(f *fakeService) { f.down, f.autoconnect = false, true })
	poll()
	if c := svc.takeCalls(); !slices.Contains(c, "settings") {
		t.Fatalf("not read when the service came back: %v", c)
	}
	if !v.autoconnect.Checked || v.autoconnect.Disabled() {
		t.Fatalf("checked %v, disabled %v", v.autoconnect.Checked, v.autoconnect.Disabled())
	}

	// Down again: disabled, and read afresh when it's back.
	svc.set(func(f *fakeService) { f.down = true })
	poll()
	if !v.autoconnect.Disabled() || v.autoconnectNote.Text != noteUnreachable {
		t.Fatal("still enabled with the service down")
	}
	svc.set(func(f *fakeService) { f.down = false })
	svc.takeCalls()
	poll()
	if !slices.Contains(svc.takeCalls(), "settings") {
		t.Fatal("not read again")
	}
}

func TestSettingsAutoconnectReadFails(t *testing.T) {
	svc := newFakeService()
	svc.settingsErr = &api.Error{Code: api.CodeInternal, Message: "broken"}
	m, poll := testWindow(t, svc)
	poll()
	selectSettings(t, m)
	v := m.settings
	if !v.autoconnect.Disabled() || !v.autoconnectNote.Visible() || v.autoconnectNote.Text == noteChecking {
		t.Fatalf("disabled %v, note %q", v.autoconnect.Disabled(), v.autoconnectNote.Text)
	}
	svc.takeCalls()
	poll()
	if slices.Contains(svc.takeCalls(), "settings") {
		t.Fatal("a failed read is retried on every poll")
	}
}

func TestSettingsAutostart(t *testing.T) {
	m, _, _ := trayWindow(t, newFakeService())
	as := &fakeAutostart{}
	m.setAutostartEntry(as, false)
	v := m.settings
	selectSettings(t, m)
	if !v.autostartBox.Visible() || v.autostart.Checked || v.autostart.Disabled() {
		t.Fatal("start at sign-in should show, off")
	}

	test.Tap(v.autostart)
	if !as.on || !v.autostart.Checked {
		t.Fatal("not on after the tap")
	}
	test.Tap(v.autostart)
	if as.on || v.autostart.Checked {
		t.Fatal("not off after the second tap")
	}

	// Removed by hand: shown when the tab is opened again.
	as.on = true
	m.tabs.SelectIndex(0)
	selectSettings(t, m)
	if !v.autostart.Checked {
		t.Fatal("an entry made elsewhere didn't show")
	}

	as.err = errors.New("permission denied")
	test.Tap(v.autostart)
	if !v.autostart.Checked || v.autostartNote.Text != "Couldn't change this setting: permission denied" {
		t.Fatalf("after a failed change: checked %v, note %q", v.autostart.Checked, v.autostartNote.Text)
	}
}

func TestSettingsAutostartHiddenWithoutTray(t *testing.T) {
	m, _ := testWindow(t, newFakeService())
	m.setAutostartEntry(&fakeAutostart{on: true}, false)
	selectSettings(t, m)
	if m.settings.autostartBox.Visible() {
		t.Fatal("start at sign-in shown without a tray")
	}
}

func TestSettingsAutostartTempBuild(t *testing.T) {
	m, _, _ := trayWindow(t, newFakeService())
	as := &fakeAutostart{}
	m.setAutostartEntry(as, true)
	m.applyAutostartDefault()
	selectSettings(t, m)
	v := m.settings
	if len(as.sets) != 0 || !v.autostart.Disabled() || v.autostartNote.Text != noteTempBuild {
		t.Fatalf("sets %v, disabled %v, note %q", as.sets, v.autostart.Disabled(), v.autostartNote.Text)
	}
	if m.a.Preferences().Bool(prefAutostartDefault) {
		t.Fatal("a temporary build used up the first-run default")
	}
}

func TestAutostartDefault(t *testing.T) {
	m, _, _ := trayWindow(t, newFakeService())
	as := &fakeAutostart{}
	m.setAutostartEntry(as, false)
	m.applyAutostartDefault()
	if !as.on || !slices.Contains(msgs(m.log), "start at sign-in turned on, the default") {
		t.Fatalf("not turned on: %v", msgs(m.log))
	}
	p := m.a.Preferences()
	if !p.Bool(prefAutostartDefault) || !p.Bool(prefAutostartNotice) {
		t.Fatal("preferences not set")
	}

	// Said on the Settings tab the first time it's opened, and not when the
	// app next starts.
	selectSettings(t, m)
	if m.settings.autostartNote.Text != noteTurnedOn {
		t.Fatalf("note %q", m.settings.autostartNote.Text)
	}
	if p.Bool(prefAutostartNotice) {
		t.Fatal("the notice would show again next time")
	}

	// Turned off, it stays off on the next start.
	test.Tap(m.settings.autostart)
	if as.on || m.settings.autostartNote.Visible() {
		t.Fatal("not off, or the notice still shows")
	}
	m.applyAutostartDefault()
	if as.on {
		t.Fatal("the default was applied twice")
	}
}

func TestAutostartDefaultLeavesExistingEntry(t *testing.T) {
	m, _, _ := trayWindow(t, newFakeService())
	as := &fakeAutostart{on: true}
	m.setAutostartEntry(as, false)
	m.applyAutostartDefault()
	if len(as.sets) != 0 || m.a.Preferences().Bool(prefAutostartNotice) {
		t.Fatalf("sets %v", as.sets)
	}
}

func TestAutostartDefaultNeedsTray(t *testing.T) {
	m, _ := testWindow(t, newFakeService())
	as := &fakeAutostart{}
	m.setAutostartEntry(as, false)
	m.applyAutostartDefault()
	if len(as.sets) != 0 || m.a.Preferences().Bool(prefAutostartDefault) {
		t.Fatal("applied without a tray")
	}
}
