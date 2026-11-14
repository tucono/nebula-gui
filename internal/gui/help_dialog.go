// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"os"
	"os/user"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	helpExplain = "This report helps someone figure out what's wrong. It's safe to paste " +
		"into an AI assistant like Claude: personal network details are hidden."
	helpDetailsLabel = "Include network details"
	helpDetailsNote  = "Only turn this on when sending the report to the person who set up your network."
	helpCollecting   = "Collecting the details…"
)

// helpDialog is the Get help dialog: an explanation, a preview of the
// troubleshooting report, the Include network details checkbox, and Copy
// and Save as file…. Only the UI goroutine touches it.
type helpDialog struct {
	m  *mainWindow
	in reportInput
	// ready is set once the diagnostics have been read, or failed.
	ready bool
	// text is the report shown, or "" while collecting or if it failed.
	text string

	preview *widget.Label
	details *widget.Check
	copyB   *widget.Button
	saveB   *widget.Button
	d       *dialog.CustomDialog
}

// openHelp shows the Get help dialog for p, which may be nil, and reads
// the service's diagnostics for it.
func (m *mainWindow) openHelp(p *problem) {
	if m.help != nil {
		return
	}
	attrs := []string{}
	if p != nil {
		attrs = append(attrs, "problem", string(p.ID))
	}
	m.log.Add("get help opened", attrs...)

	home, _ := os.UserHomeDir()
	var login string
	if u, err := user.Current(); err == nil {
		login = u.Username
	}
	h := &helpDialog{m: m, in: reportInput{
		Now:        time.Now(),
		AppVersion: m.appVersion,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Version:    m.version,
		Problem:    p,
		Status:     m.in.Status,
		Events:     m.log.Recent(reportEvents),
		Mask:       true,
		Home:       home,
		User:       login,
	}}
	h.build()
	m.help = h
	h.d.SetOnClosed(func() {
		if m.help == h {
			m.help = nil
		}
	})
	h.d.Show()
	m.fitHelp()

	ctx := m.ctx
	m.spawn(func() {
		ctx, cancel := context.WithTimeout(ctx, actionTimeout)
		defer cancel()
		diag, err := m.c.Diagnostics(ctx, api.DefaultDiagnosticsLines)
		fyne.Do(func() {
			if diag != nil {
				m.remember(namesFromStatus(&diag.Status))
				m.remember(namesFromSummary(diag.ConfigSummary))
			} else {
				m.log.Add("diagnostics failed", "error", errorDetails(err))
			}
			h.setDiagnostics(diag, err, m.names.Names())
		})
	})
}

// bannerProblem is the problem the window shows, for Get help… opened from
// the menu or the Logs tab; nil when there's none.
func (m *mainWindow) bannerProblem() *problem {
	if p := m.banner.Problem(); p != nil && !p.Info {
		return p
	}
	return nil
}

// fitHelp sizes the open Get help dialog to the window, like the picker.
func (m *mainWindow) fitHelp() {
	win := m.w.Canvas().Size()
	if m.help == nil || win.IsZero() {
		return
	}
	m.help.d.Resize(win.SubtractWidthHeight(2*pickerMargin, 2*pickerMargin))
}

func (h *helpDialog) build() {
	explain := widget.NewLabel(helpExplain)
	explain.Wrapping = fyne.TextWrapWord
	h.details = widget.NewCheck(helpDetailsLabel, func(on bool) {
		h.in.Mask = !on
		h.m.log.Add("report masking changed", "masking", onOff(!on))
		h.render()
	})
	note := widget.NewLabel(helpDetailsNote)
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.LowImportance

	h.preview = widget.NewLabel(helpCollecting)
	h.preview.TextStyle = fyne.TextStyle{Monospace: true}
	h.preview.Wrapping = fyne.TextWrapBreak
	h.preview.Selectable = true
	// The scroll's min size is small, so a long report never grows the
	// dialog past the window (see the note on the Status tab's scroll).
	scroll := container.NewVScroll(h.preview)

	h.copyB = widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), h.copyReport)
	h.copyB.Importance = widget.HighImportance
	h.saveB = widget.NewButtonWithIcon("Save as file…", theme.DocumentSaveIcon(), h.saveReport)

	top := container.NewVBox(explain, h.details, note)
	bottom := container.NewHBox(h.copyB, h.saveB)
	content := container.NewBorder(top, bottom, nil, nil, scroll)
	h.d = dialog.NewCustom("Get help", "Close", content, h.m.w)
	h.render()
}

// setDiagnostics adds the service's answer and shows the report.
func (h *helpDialog) setDiagnostics(diag *api.DiagnosticsResponse, err error, names rememberedNames) {
	h.in.Diag, h.in.DiagErr, h.in.Names = diag, err, names
	h.ready = true
	h.render()
}

// render builds the report from the input and shows it.
func (h *helpDialog) render() {
	h.text = ""
	h.copyB.SetText("Copy")
	if !h.ready {
		h.preview.SetText(helpCollecting)
		h.copyB.Disable()
		h.saveB.Disable()
		return
	}
	text, err := buildReport(h.in)
	if err != nil {
		h.m.log.Add("report refused", "error", err.Error())
		h.preview.SetText(err.Error())
		h.copyB.Disable()
		h.saveB.Disable()
		return
	}
	h.text = text
	h.preview.SetText(text)
	h.copyB.Enable()
	h.saveB.Enable()
}

func (h *helpDialog) copyReport() {
	if h.text == "" {
		return
	}
	fyne.CurrentApp().Clipboard().SetContent(h.text)
	h.copyB.SetText("Copied")
	h.m.log.Add("report copied", "masking", onOff(h.in.Mask))
}

// reportFileName is the name Save as file… suggests.
func reportFileName(now time.Time) string {
	return "nebula-report-" + now.Format(time.DateOnly) + ".md"
}

func (h *helpDialog) saveReport() {
	if h.text == "" {
		return
	}
	text, masked, m := h.text, h.in.Mask, h.m
	m.pickFile(openRequest{
		Save:       true,
		Title:      "Save report",
		FileName:   reportFileName(h.in.Now),
		FilterName: "Markdown files",
		Extensions: []string{".md"},
	}, func(path string) {
		home, _ := os.UserHomeDir()
		if err := writePrivateFile(path, []byte(text)); err != nil {
			m.log.Add("report save failed", "file", logPath(path, home), "error", err.Error())
			dialog.ShowError(err, m.w)
			return
		}
		m.log.Add("report saved", "file", logPath(path, home), "masking", onOff(masked))
	})
}
