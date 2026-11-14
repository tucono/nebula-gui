// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/storage"
)

// openRequest describes a file to ask the user for: one to open, or with
// Save set, where to save one.
type openRequest struct {
	// Save asks for a file to write instead of one to read.
	Save bool
	// FileName is the name a save picker suggests.
	FileName string
	// FilterName describes Extensions in the picker; empty means
	// "Config files".
	FilterName string
	// Title is the picker's title, where the picker shows one.
	Title string
	// Extensions limits the files offered, such as ".yml". Nil offers
	// every file.
	Extensions []string
	// StartDir is the folder the picker opens in; empty leaves it to the
	// picker.
	StartDir string
}

// nativeOpenFunc shows the operating system's file picker and blocks until
// the user closes it, or until ctx ends: then it closes the picker and
// returns ctx's error. It returns the chosen path, or "" if the user
// cancelled. An errNoNativePicker means the native picker isn't available
// and the app should use its own. parent identifies the app's window to
// the OS, or is empty.
type nativeOpenFunc func(ctx context.Context, parent string, req openRequest) (string, error)

// errNoNativePicker wraps the reason the native picker couldn't be shown.
type errNoNativePicker struct{ Err error }

func (e *errNoNativePicker) Error() string {
	return "native file picker not available: " + e.Err.Error()
}
func (e *errNoNativePicker) Unwrap() error { return e.Err }

// pickFile asks the user for a file and calls done on the UI goroutine
// with its path. It isn't called if the user cancels. It uses the native
// picker where there is one (platformOpen, set per OS), and Fyne's otherwise
// or when the native one fails.
func (m *mainWindow) pickFile(req openRequest, done func(path string)) {
	if m.picking {
		return
	}
	if m.nativeOpen == nil || m.noNative {
		m.fynePicker(req, done)
		return
	}
	m.picking = true
	parent := m.windowHandle()
	open, ctx := m.nativeOpen, m.ctx
	m.pickers.Add(1)
	m.spawn(func() {
		defer m.pickers.Done()
		path, err := open(ctx, parent, req)
		if ctx.Err() != nil {
			return // the app is closing; there's no window to update
		}
		fyne.Do(func() {
			m.picking = false
			var nn *errNoNativePicker
			switch {
			case errors.As(err, &nn):
				// Don't try again in this session: every attempt would
				// wait for the same failure first.
				m.noNative = true
				m.log.Add("native file picker not available", "error", err.Error())
				m.fynePicker(req, done)
			case err != nil:
				m.log.Add("file picker failed", "error", err.Error())
				m.fynePicker(req, done)
			case path != "":
				done(path)
			}
		})
	})
}

// fynePicker shows Fyne's picker, which is drawn inside the window.
func (m *mainWindow) fynePicker(req openRequest, done func(path string)) {
	if req.Save {
		m.fyneSavePicker(req, done)
		return
	}
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			m.log.Add("file picker failed", "error", err.Error())
			dialog.ShowError(err, m.w)
			return
		}
		if r == nil {
			return // cancelled
		}
		path := r.URI().Path()
		_ = r.Close()
		done(path)
	}, m.w)
	if req.Extensions != nil {
		d.SetFilter(storage.NewExtensionFileFilter(req.Extensions))
	}
	if req.StartDir != "" {
		if l, err := storage.ListerForURI(storage.NewFileURI(req.StartDir)); err == nil {
			d.SetLocation(l)
		}
	}
	m.showPicker(d)
}

// fyneSavePicker is Fyne's picker for a file to save. Fyne creates the
// file when the user picks it; the caller writes it again by path, with
// its own permissions.
func (m *mainWindow) fyneSavePicker(req openRequest, done func(path string)) {
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			m.log.Add("file picker failed", "error", err.Error())
			dialog.ShowError(err, m.w)
			return
		}
		if w == nil {
			return // cancelled
		}
		path := w.URI().Path()
		_ = w.Close()
		done(path)
	}, m.w)
	d.SetFileName(req.FileName)
	if req.Extensions != nil {
		d.SetFilter(storage.NewExtensionFileFilter(req.Extensions))
	}
	if req.StartDir != "" {
		if l, err := storage.ListerForURI(storage.NewFileURI(req.StartDir)); err == nil {
			d.SetLocation(l)
		}
	}
	m.showPicker(d)
}

// windowHandle identifies the window to the OS, so the native picker can
// sit on top of it. It's empty where there's no handle to give: the XDG
// portal takes "x11:<id>", and a Wayland handle needs the xdg_foreign
// protocol, which Fyne doesn't offer. The picker still works without it,
// as a separate window.
func (m *mainWindow) windowHandle() string {
	nw, ok := m.w.(driver.NativeWindow)
	if !ok {
		return ""
	}
	var h string
	nw.RunNative(func(ctx any) {
		if x, ok := ctx.(driver.X11WindowContext); ok && x.WindowHandle != 0 {
			h = fmt.Sprintf("x11:%x", x.WindowHandle)
		}
	})
	return h
}

// fileName is the last element of a picked path, for messages and the log.
func fileName(path string) string { return filepath.Base(path) }
