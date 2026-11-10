// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package gui is the Fyne user interface. It talks to the service only
// through internal/api and never imports internal/service.
package gui

import (
	"context"
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

	message := widget.NewLabel("Contacting the service…")
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

	var retry *widget.Button
	refresh := func() {
		retry.Disable()
		message.SetText("Contacting the service…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()
			v, err := opts.Client.Version(ctx)
			vv := describeVersion(opts.Client.Addr(), v, err)
			fyne.Do(func() {
				message.SetText(vv.Message)
				serviceVer.SetText(vv.ServiceVer)
				nebulaVer.SetText(vv.NebulaVer)
				apiVer.SetText(vv.APIVer)
				if vv.ShowVersions {
					versions.Show()
				} else {
					versions.Hide()
				}
				retry.Enable()
			})
		}()
	}
	retry = widget.NewButton("Retry", refresh)

	w.SetContent(container.NewPadded(container.NewVBox(message, versions, retry)))
	w.Resize(fyne.NewSize(480, 260))
	refresh()
	w.ShowAndRun()
}
