// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	logNote       = "These are technical details for troubleshooting."
	logDetailHint = "Select a line to see all of it here."
	// logDetailLines is how many lines of text the full-line pane shows
	// before it scrolls.
	logDetailLines = 3
)

// logView is the Logs tab: a note, the Follow and filter checkboxes, the
// lines in a list, and below them the whole of the selected line, which
// the list cuts off at the window's edge. Only the UI goroutine touches
// it.
type logView struct {
	model    *logModel
	list     *widget.List
	follow   *widget.Check
	warnOnly *widget.Check
	state    *widget.Label
	detail   *widget.Label
	root     fyne.CanvasObject
}

func newLogView(model *logModel) *logView {
	v := &logView{model: model}

	note := widget.NewLabel(logNote)
	note.Wrapping = fyne.TextWrapWord

	v.follow = widget.NewCheck("Follow", func(on bool) {
		if on {
			v.list.ScrollToBottom()
		}
	})
	v.follow.Checked = true // set directly: the list doesn't exist yet
	v.warnOnly = widget.NewCheck("Warnings and errors only", func(on bool) {
		v.model.setWarnOnly(on)
		v.list.UnselectAll()
		v.showDetail("")
		v.refresh()
	})

	v.state = widget.NewLabel("")
	v.state.Truncation = fyne.TextTruncateEllipsis
	v.state.Importance = widget.LowImportance

	v.list = widget.NewList(
		v.model.Len,
		func() fyne.CanvasObject {
			l := widget.NewLabel("")
			l.TextStyle = fyne.TextStyle{Monospace: true}
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(v.model.Text(i))
		},
	)
	v.list.OnSelected = func(i widget.ListItemID) {
		v.showDetail(v.model.Text(i))
	}

	v.detail = widget.NewLabel(logDetailHint)
	v.detail.TextStyle = fyne.TextStyle{Monospace: true}
	v.detail.Wrapping = fyne.TextWrapBreak
	v.detail.Selectable = true
	detailScroll := container.NewVScroll(v.detail)
	// A fixed height, so a long line scrolls instead of growing the
	// window (see the note on the Status tab's scroll).
	lineH := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{Monospace: true}).Height
	detailScroll.SetMinSize(fyne.NewSize(0, logDetailLines*lineH+2*theme.Padding()))

	top := container.NewVBox(
		note,
		container.NewHBox(v.follow, v.warnOnly),
		v.state,
	)
	bottom := container.NewVBox(widget.NewSeparator(), detailScroll)
	v.root = container.NewPadded(container.NewBorder(top, bottom, nil, nil, v.list))
	v.refresh()
	return v
}

// apply adds an update from the log poller and redraws.
func (v *logView) apply(u logUpdate) {
	if v.model.apply(u) {
		// The selected index now points at another line; the pane keeps
		// the text it shows.
		v.list.UnselectAll()
	}
	v.refresh()
}

func (v *logView) refresh() {
	// The label stays even when empty, so the list doesn't jump.
	v.state.SetText(v.model.State())
	v.list.Refresh()
	if v.follow.Checked {
		v.list.ScrollToBottom()
	}
}

func (v *logView) showDetail(text string) {
	if text == "" {
		text = logDetailHint
	}
	v.detail.SetText(text)
}
