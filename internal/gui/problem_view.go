// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// problemHandlers connects a problem view to the window.
type problemHandlers struct {
	// OnFix runs the fix button. Nil hides fix buttons.
	OnFix func(fixAction)
	// OnHelp opens the troubleshooting report for the problem. Nil hides
	// Get help… until the report exists (M2 step 9).
	OnHelp func(*problem)
	// Versions returns the app and service versions for Details.
	Versions func() string
}

// problemView shows one problem: title, explanation, numbered steps, an
// optional command with Copy, the fix button, a collapsed Details section
// and Get help…. The same view is used as the window's banner and inside
// dialogs.
type problemView struct {
	h   problemHandlers
	box *fyne.Container
	cur *problem
}

func newProblemView(h problemHandlers) *problemView {
	v := &problemView{h: h, box: container.NewVBox()}
	v.box.Hide()
	return v
}

// Object is the view's canvas object; it's hidden while no problem is set.
func (v *problemView) Object() fyne.CanvasObject { return v.box }

// Problem returns the problem shown, or nil.
func (v *problemView) Problem() *problem { return v.cur }

// Set shows p, or hides the view for nil. Call it on the UI goroutine.
func (v *problemView) Set(p *problem) {
	v.cur = p
	if p == nil {
		v.box.Objects = nil
		v.box.Hide()
		return
	}
	v.box.Objects = problemObjects(p, v.h)
	v.box.Refresh()
	v.box.Show()
}

func problemObjects(p *problem, h problemHandlers) []fyne.CanvasObject {
	icon := theme.ErrorIcon()
	if p.Info {
		icon = theme.InfoIcon()
	}
	title := widget.NewLabelWithStyle(p.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapWord
	objs := []fyne.CanvasObject{
		container.NewBorder(nil, nil, widget.NewIcon(icon), nil, title),
	}
	if p.Explain != "" {
		objs = append(objs, wrapped(p.Explain))
	}
	if len(p.Steps) > 0 {
		objs = append(objs, wrapped(numberSteps(p.Steps)))
	}
	if p.Command != "" {
		cmd := widget.NewLabelWithStyle(p.Command, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		cmd.Selectable = true
		cmd.Wrapping = fyne.TextWrapBreak
		copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
			fyne.CurrentApp().Clipboard().SetContent(p.Command)
		})
		objs = append(objs, container.NewBorder(nil, nil, nil, copyBtn, cmd))
	}

	var buttons []fyne.CanvasObject
	if p.Fix != nil && h.OnFix != nil {
		fix := *p.Fix
		b := widget.NewButton(fix.Label, func() { h.OnFix(fix) })
		b.Importance = widget.HighImportance
		buttons = append(buttons, b)
	}
	if h.OnHelp != nil {
		buttons = append(buttons, widget.NewButtonWithIcon("Get help…", theme.HelpIcon(), func() { h.OnHelp(p) }))
	}
	if len(buttons) > 0 {
		objs = append(objs, container.NewHBox(buttons...))
	}

	details := p.Details
	if h.Versions != nil {
		details = joinDetails(details, h.Versions())
	}
	if details != "" && !p.Info {
		d := widget.NewLabelWithStyle(details, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		d.Selectable = true
		d.Wrapping = fyne.TextWrapBreak
		objs = append(objs, widget.NewAccordion(widget.NewAccordionItem("Details", d)))
	}
	return objs
}

func wrapped(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// numberSteps writes one step as is and several as a numbered list.
func numberSteps(steps []string) string {
	if len(steps) == 1 {
		return steps[0]
	}
	var b strings.Builder
	for i, s := range steps {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d. %s", i+1, s)
	}
	return b.String()
}

// showProblemDialog shows p in a dialog, for problems that answer something
// the user just did, such as an import. A fix closes the dialog first.
func showProblemDialog(w fyne.Window, p *problem, h problemHandlers) {
	var d *dialog.CustomDialog
	if h.OnFix != nil {
		onFix := h.OnFix
		h.OnFix = func(f fixAction) {
			d.Hide()
			onFix(f)
		}
	}
	content := container.NewVBox(problemObjects(p, h)...)
	d = dialog.NewCustom("", "Close", content, w)
	d.Resize(fyne.NewSize(460, 0))
	d.Show()
}
