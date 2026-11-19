// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const (
	projectURL     = "https://github.com/tucono/nebula-gui"
	aboutSummary   = "A desktop app for joining a Nebula network."
	aboutAffiliate = "nebula-gui is an independent project. It is not affiliated with or " +
		"endorsed by Slack or Defined Networking. Nebula is copyright Slack Technologies, Inc. " +
		"and is MIT licensed."
	aboutNoNotices = "The open-source licenses come with the installed app."
	// noticesFile is what make notices writes and the packages install.
	noticesFile = "THIRD_PARTY_NOTICES.txt"
)

// noticesPath is where the package installed the third-party notices for
// a program at exe on goos.
func noticesPath(goos, exe string) string {
	switch goos {
	case "windows":
		return filepath.Join(filepath.Dir(exe), noticesFile)
	case "darwin":
		return filepath.Join(filepath.Dir(exe), "..", "Resources", noticesFile)
	default:
		return "/usr/share/doc/nebula-gui/" + noticesFile
	}
}

// installedNotices is the notices file for this program, or "" when it
// can't tell. Tests replace it.
var installedNotices = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return noticesPath(runtime.GOOS, exe)
}

// fileURL is the file:// URL for an absolute path, Windows' included.
func fileURL(path string) *url.URL {
	p := filepath.ToSlash(path)
	if p != "" && p[0] != '/' {
		p = "/" + p // C:/… becomes /C:/…
	}
	return &url.URL{Scheme: "file", Path: p}
}

// openAbout shows the About dialog: the version, the project page, the
// note that the project isn't Nebula's, and the third-party licenses.
func (m *mainWindow) openAbout() {
	m.log.Add("about opened")
	title := widget.NewLabel("nebula-gui " + m.appVersion)
	title.TextStyle = fyne.TextStyle{Bold: true}
	summary := widget.NewLabel(aboutSummary)
	summary.Wrapping = fyne.TextWrapWord
	affiliate := widget.NewLabel(aboutAffiliate)
	affiliate.Wrapping = fyne.TextWrapWord
	page, _ := url.Parse(projectURL)
	items := []fyne.CanvasObject{title, summary, widget.NewHyperlink(projectURL, page), affiliate}

	path := installedNotices()
	if _, err := os.Stat(path); path != "" && err == nil {
		items = append(items, widget.NewButton("Open-source licenses", func() {
			if err := fyne.CurrentApp().OpenURL(fileURL(path)); err != nil {
				m.log.Add("licenses open failed", "error", err.Error())
			}
		}))
	} else {
		note := widget.NewLabel(aboutNoNotices)
		note.Importance = widget.LowImportance
		items = append(items, note)
	}

	d := dialog.NewCustom("About nebula-gui", "Close", container.NewVBox(items...), m.w)
	d.Show()
	win := m.w.Canvas().Size()
	d.Resize(fyne.NewSize(min(win.Width-2*pickerMargin, 440), d.MinSize().Height))
}
