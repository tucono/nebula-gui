// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

// actionClient is the part of api.Client the window's buttons use.
type actionClient interface {
	serviceClient
	Connect(ctx context.Context) (*api.StatusResponse, error)
	Disconnect(ctx context.Context) (*api.StatusResponse, error)
	PutConfig(ctx context.Context, yaml string) (*api.ConfigPutResponse, error)
	DeleteConfig(ctx context.Context) (*api.StatusResponse, error)
	Diagnostics(ctx context.Context, lines int) (*api.DiagnosticsResponse, error)
	logsClient
	Addr() string
}

// mainWindow is the app's window. The Status tab has a problem banner or
// the status header, then the versions in small print; the Logs tab has
// the service log. Its fields are only touched on the UI
// goroutine; the poller and the actions hand results over with fyne.Do.
type mainWindow struct {
	a         fyne.App
	w         fyne.Window
	c         actionClient
	log       *appLog
	gate      *statusGate
	poller    *poller
	logPoller *logPoller
	// names are remembered for masking troubleshooting reports.
	names      *nameStore
	appVersion string
	// spawn runs an action's call off the UI goroutine. Tests replace it
	// to run the call in place.
	spawn func(func())
	// readFile reads the files a config refers to. Tests replace it.
	readFile func(string) ([]byte, error)

	in         statusInput
	version    *api.VersionResponse
	lastBanner problemID
	importing  bool

	banner   *problemView
	waiting  *widget.Label
	header   *fyne.Container
	headline *widget.Label
	spinner  *widget.Activity
	address  *widget.Label
	cert     *widget.Label
	valid    *widget.Label
	uptime   *widget.Label
	button   *widget.Button
	importB  *widget.Button
	removeB  *widget.Button
	footer   *widget.RichText
	tabs     *container.AppTabs
	logs     *logView
	// help is the open Get help dialog, or nil.
	help *helpDialog

	// picker is the open Fyne file picker, kept the size of the window.
	picker *dialog.FileDialog
	// nativeOpen shows the OS's file picker; nil uses Fyne's. Tests
	// replace it.
	nativeOpen nativeOpenFunc
	// picking is set while the native picker is open, which doesn't
	// block the window the way Fyne's modal picker does.
	picking bool
	// noNative is set once the native picker has failed to open.
	noNative bool

	// ctx ends when the window closes; it stops the poller and closes an
	// open native picker. pickers counts the native pickers still open.
	ctx     context.Context
	cancel  context.CancelFunc
	pickers sync.WaitGroup
}

func newMainWindow(a fyne.App, c actionClient, log *appLog, appVersion string) *mainWindow {
	m := &mainWindow{
		a:          a,
		w:          a.NewWindow("Nebula"),
		c:          c,
		log:        log,
		gate:       &statusGate{},
		names:      openNameStore(""),
		appVersion: appVersion,
		spawn:      func(f func()) { go f() },
		readFile:   readCapped,
		nativeOpen: platformOpen,
		in:         statusInput{GOOS: runtime.GOOS},
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.poller = newPoller(c, log, m.gate)
	m.logPoller = newLogPoller(c)
	m.logs = newLogView(newLogModel(), func() { m.openHelp(m.bannerProblem()) })

	m.banner = newProblemView(problemHandlers{
		OnFix:     m.fix,
		Supports:  supportedFix,
		OnDismiss: m.dismiss,
		OnHelp:    m.openHelp,
		Versions:  m.versionsLine,
	})
	m.waiting = widget.NewLabel("Contacting the background service…")
	m.waiting.Wrapping = fyne.TextWrapWord

	m.headline = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	m.headline.SizeName = theme.SizeNameSubHeadingText
	m.spinner = widget.NewActivity()
	m.spinner.Hide()
	m.address = widget.NewLabel("")
	m.cert = widget.NewLabel("")
	m.valid = widget.NewLabel("")
	m.uptime = widget.NewLabel("")
	m.button = widget.NewButton("Connect", func() {
		m.act(describeStatus(m.now()).ButtonAction)
	})
	m.button.Importance = widget.HighImportance
	m.importB = widget.NewButton("Import config…", m.importConfig)
	m.removeB = widget.NewButton("Remove config…", m.confirmRemove)
	m.removeB.Importance = widget.LowImportance
	m.header = container.NewVBox(
		container.NewHBox(m.headline, m.spinner),
		widget.NewForm(
			widget.NewFormItem("Address", m.address),
			widget.NewFormItem("Certificate", m.cert),
			widget.NewFormItem("Valid until", m.valid),
			widget.NewFormItem("Connected for", m.uptime),
		),
		container.NewHBox(m.button, m.importB, m.removeB),
	)
	m.header.Hide()

	m.footer = widget.NewRichText()
	m.footer.Wrapping = fyne.TextWrapWord

	// The scroll keeps the content from growing the window. Fyne enlarges
	// a window whose content no longer fits, and some Wayland compositors
	// (seen on COSMIC) ignore that, which leaves the layout taller than the
	// window and the mouse out of step with it.
	scroll := container.NewVScroll(container.NewPadded(container.NewVBox(
		m.banner.Object(),
		m.waiting,
		m.header,
		widget.NewSeparator(),
		m.footer,
	)))
	m.tabs = container.NewAppTabs(
		container.NewTabItem("Status", scroll),
		container.NewTabItem("Logs", m.logs.root),
	)
	m.tabs.OnSelected = func(*container.TabItem) {
		m.logPoller.SetFast(m.logsShowing())
	}
	m.w.SetMainMenu(m.mainMenu())
	m.w.SetContent(container.New(&resizeWatcher{onResize: func(fyne.Size) {
		m.fitPicker()
		m.fitHelp()
	}}, m.tabs))
	m.w.Resize(fyne.NewSize(520, 420))
	m.render()
	return m
}

// mainMenu is File → Quit and Help → Get help…, for when something feels
// wrong without a problem being shown. Quit is set explicitly: Fyne adds
// one to the first menu otherwise, which would put it under Help.
func (m *mainWindow) mainMenu() *fyne.MainMenu {
	quit := fyne.NewMenuItem("Quit", func() { m.a.Quit() })
	quit.IsQuit = true
	return fyne.NewMainMenu(
		fyne.NewMenu("File", quit),
		fyne.NewMenu("Help", fyne.NewMenuItem("Get help…", func() { m.openHelp(m.bannerProblem()) })),
	)
}

// remember keeps names for masking reports. Saving can fail, for example
// on a full disk; the report then masks with what it has.
func (m *mainWindow) remember(n rememberedNames) {
	if err := m.names.Remember(n); err != nil {
		m.log.Add("couldn't save names for masking", "error", err.Error())
	}
}

// now returns the input with the current time, for describeStatus.
func (m *mainWindow) now() statusInput {
	in := m.in
	in.Now = time.Now()
	return in
}

// start polls until the window closes.
func (m *mainWindow) start() {
	go m.poller.run(m.ctx, func(r pollResult) {
		fyne.Do(func() { m.applyPoll(r) })
	})
	go m.logPoller.run(m.ctx, func(u logUpdate) {
		fyne.Do(func() { m.logs.apply(u) })
	})
}

// logsShowing reports whether the Logs tab is the selected one.
func (m *mainWindow) logsShowing() bool {
	return m.tabs.Selected() != nil && m.tabs.Selected().Content == m.logs.root
}

// closeWait is how long close waits for a native picker to be closed.
const closeWait = 2 * time.Second

// close stops what the window started, after the app's loop has ended.
// The desktop's picker is a separate window that outlives the app unless
// it's told to close, so close waits briefly for that message to go out.
func (m *mainWindow) close() {
	m.cancel()
	done := make(chan struct{})
	go func() {
		m.pickers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(closeWait):
		m.log.Add("file picker didn't close in time")
	}
}

func (m *mainWindow) applyPoll(r pollResult) {
	if r.Version != nil {
		m.version = r.Version
		m.in.Version = versionProblem(r.Version)
	}
	if m.gate.Accept(r.Seq) {
		switch {
		case r.Err != nil:
			m.in.Err, m.in.Status = r.Err, nil
		case r.Status != nil:
			m.applyStatus(r.Status)
		default: // another API version: no status to show
			m.in.Err, m.in.Status = nil, nil
		}
	}
	m.render()
}

func (m *mainWindow) applyStatus(st *api.StatusResponse) {
	m.in.Err, m.in.Status = nil, st
	m.remember(namesFromStatus(st))
	if st.State == api.StateRunning || st.State == api.StateStarting {
		m.in.Action = nil // the problem it showed is over
	}
	if st.State != api.StateRunning {
		m.in.NewConfig = false // the next connect uses the new config
	}
}

// render updates the widgets from m.in.
func (m *mainWindow) render() {
	v := describeStatus(m.now())

	m.banner.Set(v.Banner)
	var id problemID
	if v.Banner != nil {
		id = v.Banner.ID
	}
	if id != m.lastBanner {
		if id != "" {
			m.log.Add("problem shown", "problem", string(id), "details", v.Banner.Details)
		}
		m.lastBanner = id
	}

	m.waiting.Hidden = !v.Waiting
	m.waiting.Refresh()

	m.header.Hidden = !v.ShowHeader
	if v.ShowHeader {
		m.headline.SetText(v.Headline)
		if v.Busy {
			m.spinner.Show()
			m.spinner.Start()
		} else {
			m.spinner.Stop()
			m.spinner.Hide()
		}
		m.address.SetText(v.Address)
		m.cert.SetText(v.Cert)
		m.valid.SetText(v.ValidUntil)
		m.uptime.SetText(v.Uptime)
		m.button.SetText(v.Button)
		setEnabled(m.button, v.ButtonEnabled)
		setEnabled(m.importB, v.ManageEnabled && !m.importing)
		setEnabled(m.removeB, v.ManageEnabled && !m.importing)
	}
	m.header.Refresh()

	m.footer.Segments = []widget.RichTextSegment{&widget.TextSegment{
		Text: m.footerText(),
		Style: widget.RichTextStyle{
			SizeName:  theme.SizeNameCaptionText,
			ColorName: theme.ColorNamePlaceHolder,
			Inline:    true,
		},
	}}
	m.footer.Refresh()
}

func (m *mainWindow) footerText() string {
	s := "App " + m.appVersion
	if m.version != nil {
		vv := describeVersion(m.version, nil)
		s += fmt.Sprintf(" · Service %s · Nebula %s · API %s", vv.ServiceVer, vv.NebulaVer, vv.APIVer)
	}
	return s
}

// versionsLine goes under Details in every problem.
func (m *mainWindow) versionsLine() string {
	svc := "—"
	if m.version != nil {
		svc = orDash(m.version.ServiceVersion)
	}
	return fmt.Sprintf("app %s, service %s, service address %s", m.appVersion, svc, m.c.Addr())
}

// resizeWatcher fills the window with its one object and reports the new
// size; Fyne has no resize callback on a window. Its min size is the
// object's, so it never asks the window to grow.
type resizeWatcher struct {
	onResize func(fyne.Size)
	last     fyne.Size
}

func (r *resizeWatcher) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
	if size != r.last {
		r.last = size
		r.onResize(size)
	}
}

func (r *resizeWatcher) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objs {
		s = s.Max(o.MinSize())
	}
	return s
}

// pickerMargin is the space left around the file picker, so the window
// behind it still shows.
const pickerMargin = 24

// showPicker opens a file picker that fills the window and follows it when
// the window is resized. Fyne's picker is drawn inside the window and
// can't be resized by itself, and its default size is small.
func (m *mainWindow) showPicker(d *dialog.FileDialog) {
	m.picker = d
	d.SetOnClosed(func() {
		if m.picker == d {
			m.picker = nil
		}
	})
	d.Show()
	m.fitPicker()
}

// fitPicker sizes the open picker to the window. Fyne keeps it at least at
// its min size, and no larger than the window.
func (m *mainWindow) fitPicker() {
	win := m.w.Canvas().Size()
	if m.picker == nil || win.IsZero() {
		return
	}
	m.picker.Resize(win.SubtractWidthHeight(2*pickerMargin, 2*pickerMargin))
}

func setEnabled(b *widget.Button, on bool) {
	if on {
		b.Enable()
	} else {
		b.Disable()
	}
}

// supportedFix lists the fixes the banner offers. The fixes that continue
// an import are in its dialog (importFixes).
func supportedFix(k fixKind) bool {
	switch k {
	case fixRetry, fixOpenDownloads, fixConnect, fixReconnect, fixImport:
		return true
	default:
		return false
	}
}

func (m *mainWindow) fix(f fixAction) {
	switch f.Kind {
	case fixRetry:
		m.poller.Poke()
		m.logPoller.Poke()
	case fixOpenDownloads:
		openDownloads(m.a)
	case fixConnect:
		m.act(actConnect)
	case fixReconnect:
		m.act(actReconnect)
	case fixImport:
		m.importConfig()
	}
}

// confirmRemove asks before deleting the stored config.
func (m *mainWindow) confirmRemove() {
	msg := "You'll need the config file again to reconnect."
	if m.in.Status != nil && m.in.Status.State == api.StateRunning {
		msg = "This disconnects you. " + msg
	}
	dialog.ShowConfirm("Remove this config?", msg, func(ok bool) {
		if ok {
			m.act(actRemoveConfig)
		}
	}, m.w)
}

// dismiss clears a problem from a failed action, and the service's error
// state with Disconnect.
func (m *mainWindow) dismiss(*problem) {
	m.in.Action = nil
	m.log.Add("problem dismissed")
	if m.in.Status != nil && m.in.Status.State == api.StateError {
		m.act(actDisconnect)
		return
	}
	m.render()
}

// act runs a Connect or Disconnect off the UI goroutine. The button stays
// disabled until it returns; polling goes on meanwhile, so the header can
// show "Disconnecting…".
func (m *mainWindow) act(a action) {
	if a == actNone || m.in.Acting != actNone || m.importing {
		return
	}
	m.in.Acting, m.in.Action = a, nil
	m.log.Add(a.String() + " clicked")
	m.render()
	var cert *api.CertInfo
	if m.in.Status != nil {
		cert = m.in.Status.Cert
	}

	m.spawn(func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		st, err := runAction(ctx, m.c, a)
		seq := m.gate.Begin() // the response shows the state after the action
		fyne.Do(func() {
			m.in.Acting = actNone
			if err != nil {
				p := problemFor(err, problemContext{Op: a.op(), Cert: cert, GOOS: m.in.GOOS})
				p.Dismiss = true
				m.in.Action = p
				m.log.Add(a.String()+" failed", "problem", string(p.ID), "error", errorDetails(err))
				m.poller.Poke() // the status may have changed too
			} else {
				m.log.Add(a.String()+" done", "state", string(st.State))
				switch a {
				case actReconnect:
					m.in.NewConfig = false // running with the new config now
				case actRemoveConfig:
					if err := m.names.Clear(); err != nil {
						m.log.Add("couldn't forget names for masking", "error", err.Error())
					}
				}
				if m.gate.Accept(seq) {
					m.applyStatus(st)
				}
			}
			m.render()
		})
	})
}

// runAction calls the service for a. Reconnect disconnects first; if
// Nebula had already stopped, that isn't an error.
func runAction(ctx context.Context, c actionClient, a action) (*api.StatusResponse, error) {
	switch a {
	case actConnect:
		return c.Connect(ctx)
	case actDisconnect:
		return c.Disconnect(ctx)
	case actReconnect:
		if _, err := c.Disconnect(ctx); err != nil && api.ErrorCodeOf(err) != api.CodeNotRunning {
			return nil, err
		}
		return c.Connect(ctx)
	case actRemoveConfig:
		return c.DeleteConfig(ctx)
	default:
		return nil, fmt.Errorf("unknown action %d", a)
	}
}
