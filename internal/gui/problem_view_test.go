// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// find returns the first object in the tree that matches.
func find[T fyne.CanvasObject](t *testing.T, o fyne.CanvasObject, match func(T) bool) (T, bool) {
	if v, ok := o.(T); ok && match(v) {
		return v, true
	}
	var objs []fyne.CanvasObject
	switch c := o.(type) {
	case *fyne.Container:
		objs = c.Objects
	case *widget.Accordion:
		for _, it := range c.Items {
			objs = append(objs, it.Detail)
		}
	case fyne.Widget:
		// Dialogs and pop-ups hold their content in their renderers.
		objs = test.TempWidgetRenderer(t, c).Objects()
	}
	for _, child := range objs {
		if v, ok := find(t, child, match); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func button(t *testing.T, o fyne.CanvasObject, text string) *widget.Button {
	t.Helper()
	b, _ := find(t, o, func(b *widget.Button) bool { return b.Text == text })
	return b
}

func label(t *testing.T, o fyne.CanvasObject, text string) *widget.Label {
	t.Helper()
	l, _ := find(t, o, func(l *widget.Label) bool { return l.Text == text })
	return l
}

func TestProblemView(t *testing.T) {
	test.NewTempApp(t)
	var fixed []fixAction
	var helped *problem
	v := newProblemView(problemHandlers{
		OnFix:    func(f fixAction) { fixed = append(fixed, f) },
		OnHelp:   func(p *problem) { helped = p },
		Versions: func() string { return "app 1.0, service 1.0" },
	})
	if v.Object().Visible() {
		t.Fatal("visible without a problem")
	}

	p := fileMissing("key", "laptop.key")
	p.Details = "raw error"
	v.Set(p)
	o := v.Object()
	if !o.Visible() || v.Problem() != p {
		t.Fatal("problem not shown")
	}
	if label(t, o, p.Title) == nil || label(t, o, p.Explain) == nil || label(t, o, p.Steps[0]) == nil {
		t.Fatal("title, explanation or step missing")
	}
	if label(t, o, "raw error\napp 1.0, service 1.0") == nil {
		t.Fatal("details missing the raw error and versions")
	}
	test.Tap(button(t, o, "Choose the file…"))
	if len(fixed) != 1 || fixed[0].Kind != fixChooseFile || fixed[0].PKIKey != "key" {
		t.Fatalf("fix = %+v", fixed)
	}
	test.Tap(button(t, o, "Get help…"))
	if helped != p {
		t.Fatal("Get help… didn't pass the problem")
	}

	v.Set(noPermission("linux"))
	o = v.Object()
	if button(t, o, "Choose the file…") != nil {
		t.Fatal("old fix button left behind")
	}
	cmd := noPermission("linux").Command
	test.Tap(button(t, o, "Copy"))
	if got := fyne.CurrentApp().Clipboard().Content(); got != cmd {
		t.Fatalf("clipboard = %q, want %q", got, cmd)
	}

	v.Set(nil)
	if v.Object().Visible() || v.Problem() != nil {
		t.Fatal("still visible after Set(nil)")
	}
}

func TestProblemViewWithoutHandlers(t *testing.T) {
	test.NewTempApp(t)
	v := newProblemView(problemHandlers{})
	v.Set(serviceNotRunning())
	if button(t, v.Object(), "Try again") != nil || button(t, v.Object(), "Get help…") != nil {
		t.Fatal("buttons shown without handlers")
	}
}

func TestProblemDialog(t *testing.T) {
	test.NewTempApp(t)
	w := test.NewTempWindow(t, widget.NewLabel("main"))
	w.Resize(fyne.NewSize(600, 400))
	var fixed []fixAction
	showProblemDialog(w, sshdEnabled(), problemHandlers{OnFix: func(f fixAction) { fixed = append(fixed, f) }})

	overlay := w.Canvas().Overlays().Top()
	if overlay == nil {
		t.Fatal("no dialog")
	}
	b := button(t, overlay, "Turn it off and import")
	if b == nil {
		t.Fatal("no fix button in the dialog")
	}
	test.Tap(b)
	if len(fixed) != 1 || fixed[0].Kind != fixDisableSSHD {
		t.Fatalf("fix = %+v", fixed)
	}
	if w.Canvas().Overlays().Top() != nil {
		t.Fatal("dialog still open after the fix")
	}
}
