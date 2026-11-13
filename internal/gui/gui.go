// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package gui is the Fyne user interface. It talks to the service only
// through internal/api and never imports internal/service.
package gui

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	appID          = "io.github.tucono.nebulagui"
	requestTimeout = 5 * time.Second
)

// Options configures the GUI.
type Options struct {
	Client     *api.Client
	AppVersion string
}

// Run opens the main window and blocks until the app quits.
func Run(opts Options) {
	a := app.NewWithID(appID)
	w := a.NewWindow("Nebula")

	message := widget.NewLabel("Contacting the background service…")
	message.Wrapping = fyne.TextWrapWord
	serviceVer := widget.NewLabel("")
	nebulaVer := widget.NewLabel("")
	apiVer := widget.NewLabel("")
	versions := widget.NewForm(
		widget.NewFormItem("App", widget.NewLabel(opts.AppVersion)),
		widget.NewFormItem("Service", serviceVer),
		widget.NewFormItem("Nebula", nebulaVer),
		widget.NewFormItem("API", apiVer),
	)
	versions.Hide()

	var refresh func()
	banner := newProblemView(problemHandlers{
		OnFix: func(f fixAction) {
			switch f.Kind {
			case fixRetry:
				refresh()
			case fixOpenDownloads:
				openDownloads(a)
			}
		},
		Versions: func() string {
			return fmt.Sprintf("app %s, service %s, service address %s",
				opts.AppVersion, orDash(serviceVer.Text), opts.Client.Addr())
		},
	})

	refresh = func() {
		// While a problem is shown it stays put during the retry. Showing
		// the message on top of it would make the content taller than the
		// window for a moment.
		if banner.Problem() == nil {
			message.SetText("Contacting the background service…")
			message.Show()
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			v, err := opts.Client.Version(ctx)
			vv := describeVersion(v, err)
			fyne.Do(func() {
				serviceVer.SetText(vv.ServiceVer)
				nebulaVer.SetText(vv.NebulaVer)
				apiVer.SetText(vv.APIVer)
				if vv.ShowVersions {
					versions.Show()
				} else {
					versions.Hide()
				}
				banner.Set(vv.Problem)
				message.SetText("The background service is running.")
				message.Hidden = vv.Problem != nil
				message.Refresh()
			})
		}()
	}

	// The scroll keeps the content from growing the window. Fyne enlarges
	// a window whose content no longer fits, and some Wayland compositors
	// (seen on COSMIC) ignore that, which leaves the layout taller than the
	// window and the mouse out of step with it.
	w.SetContent(container.NewVScroll(container.NewPadded(container.NewVBox(banner.Object(), message, versions))))
	w.Resize(fyne.NewSize(480, 260))
	refresh()
	w.ShowAndRun()
}

// openDownloads opens the public releases page in the browser.
func openDownloads(a fyne.App) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return
	}
	_ = a.OpenURL(u)
}
