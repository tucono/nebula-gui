// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package gui is the Fyne user interface. It talks to the service only
// through internal/api and never imports internal/service.
package gui

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/autostart"
	"github.com/tucono/nebula-gui/internal/userfiles"
)

const appID = userfiles.AppID

// Options configures the GUI.
type Options struct {
	Client     *api.Client
	AppVersion string
	// Hidden starts the app in the tray without showing the window, for
	// starting at sign-in. Without a tray the window shows anyway.
	Hidden bool
	// OtherAddress is set when Client doesn't use the installed service's
	// address, so the installed service says nothing about why it can't
	// be reached.
	OtherAddress bool
}

// Run opens the main window and blocks until the app quits.
func Run(opts Options) {
	inst, instErr := claimDefaultInstance(!opts.Hidden)
	if errors.Is(instErr, errAlreadyRunning) {
		fmt.Fprintln(os.Stderr, "nebula-gui: already running")
		return
	}

	log := newAppLog()
	if dir, err := defaultAppLogDir(); err == nil {
		if l, err := openAppLog(dir); err != nil {
			fmt.Fprintf(os.Stderr, "nebula-gui: GUI log kept in memory only: %v\n", err)
		} else {
			log = l
		}
	}
	defer func() { _ = log.Close() }()
	log.Add("app started", "version", opts.AppVersion, "os", runtime.GOOS+"/"+runtime.GOARCH)
	if instErr != nil {
		log.Add("can't check for another copy", "error", instErr.Error())
	}

	a := app.NewWithID(appID)
	a.SetIcon(appIcon)
	m := newMainWindow(a, opts.Client, log, opts.AppVersion)
	if p, err := defaultNameStorePath(); err == nil {
		m.names = openNameStore(p)
	}
	if p, err := defaultPeerStorePath(); err == nil {
		m.saved = openPeerStore(p)
	}
	if !opts.OtherAddress {
		m.poller.probe = probeSetup
	}
	m.start()
	if inst != nil {
		go inst.serve(func() {
			fyne.Do(func() {
				m.log.Add("another copy asked to show the window")
				m.showWindow()
			})
		})
	}
	a.Lifecycle().SetOnStarted(func() {
		watchReopen(func() {
			fyne.Do(func() {
				m.log.Add("app opened again: showing the window")
				m.showWindow()
			})
		})
	})
	if t := newDesktopTray(a); t != nil {
		m.attachTray(t)
	} else {
		log.Add("no tray: closing the window quits")
	}
	setUpAutostart(m)
	if opts.Hidden && m.tray != nil {
		m.setHidden(true)
		log.Add("started hidden in the tray")
		a.Run()
	} else {
		m.w.ShowAndRun()
	}
	if inst != nil {
		_ = inst.Close()
	}
	m.close()
	log.Add("app closed")
}

// claimDefaultInstance claims this user's instance address.
func claimDefaultInstance(show bool) (*instance, error) {
	addr, err := defaultInstanceAddr()
	if err != nil {
		return nil, err
	}
	return claimInstance(addr, show)
}

// setUpAutostart gives the window this user's start-at-sign-in entry, and
// on the first run with a tray turns it on.
func setUpAutostart(m *mainWindow) {
	exe, err := os.Executable()
	if err != nil {
		m.log.Add("start at sign-in unavailable", "error", err.Error())
		return
	}
	e, err := autostart.New(exe)
	if err != nil {
		m.log.Add("start at sign-in unavailable", "error", err.Error())
		return
	}
	m.setAutostartEntry(e, autostart.IsTempBuild(exe))
	m.applyAutostartDefault()
}

// openDownloads opens the public releases page in the browser.
func openDownloads(a fyne.App) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return
	}
	_ = a.OpenURL(u)
}
