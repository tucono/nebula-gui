// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package gui is the Fyne user interface. It talks to the service only
// through internal/api and never imports internal/service.
package gui

import (
	"fmt"
	"net/url"
	"os"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"github.com/tucono/nebula-gui/internal/api"
)

const appID = "io.github.tucono.nebulagui"

// Options configures the GUI.
type Options struct {
	Client     *api.Client
	AppVersion string
	// Hidden starts the app in the tray without showing the window, for
	// starting at sign-in. Without a tray the window shows anyway.
	Hidden bool
}

// Run opens the main window and blocks until the app quits.
func Run(opts Options) {
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

	a := app.NewWithID(appID)
	a.SetIcon(appIcon)
	m := newMainWindow(a, opts.Client, log, opts.AppVersion)
	if p, err := defaultNameStorePath(); err == nil {
		m.names = openNameStore(p)
	}
	if p, err := defaultPeerStorePath(); err == nil {
		m.saved = openPeerStore(p)
	}
	m.start()
	if t := newDesktopTray(a); t != nil {
		m.attachTray(t)
	} else {
		log.Add("no tray: closing the window quits")
	}
	if opts.Hidden && m.tray != nil {
		m.setHidden(true)
		log.Add("started hidden in the tray")
		a.Run()
	} else {
		m.w.ShowAndRun()
	}
	m.close()
	log.Add("app closed")
}

// openDownloads opens the public releases page in the browser.
func openDownloads(a fyne.App) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return
	}
	_ = a.OpenURL(u)
}
