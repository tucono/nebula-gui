//go:build darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

/*
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#include <stdint.h>
#include <stdlib.h>
void nebulaPickFile(uintptr_t handle, uintptr_t window, int save, const char *title,
	const char *fileName, const char *startDir, const char *extensions);
void nebulaPickCancel(void);
*/
import "C"

import (
	"context"
	"runtime/cgo"
	"strconv"
	"strings"
	"unsafe"
)

// platformOpen is the Finder's open and save panels (picker_darwin.m),
// shown as a sheet on the window.
var platformOpen nativeOpenFunc = finderOpenFile

// macWindowPrefix starts a window handle that names an NSWindow; see
// windowHandle.
const macWindowPrefix = "nswindow:"

// finderOpenFile shows the open panel, or with req.Save set the save
// panel, and waits for the user to close it. parent names the window to
// attach it to; without one it's a separate window.
func finderOpenFile(ctx context.Context, parent string, req openRequest) (string, error) {
	var window uint64
	if h, ok := strings.CutPrefix(parent, macWindowPrefix); ok {
		window, _ = strconv.ParseUint(h, 16, 64)
	}
	exts := make([]string, 0, len(req.Extensions))
	for _, e := range req.Extensions {
		if e = strings.TrimPrefix(e, "."); e != "" {
			exts = append(exts, e)
		}
	}

	done := make(chan string, 1)
	h := cgo.NewHandle(done) // deleted by nebulaPicked
	title, name, dir := C.CString(req.Title), C.CString(req.FileName), C.CString(req.StartDir)
	ext := C.CString(strings.Join(exts, "\n"))
	defer func() {
		for _, s := range []*C.char{title, name, dir, ext} {
			C.free(unsafe.Pointer(s))
		}
	}()
	save := C.int(0)
	if req.Save {
		save = 1
	}
	C.nebulaPickFile(C.uintptr_t(h), C.uintptr_t(window), save, title, name, dir, ext)

	select {
	case path := <-done:
		return path, nil
	case <-ctx.Done():
		C.nebulaPickCancel()
		return "", ctx.Err()
	}
}

// nebulaPicked receives the panel's answer: the chosen path, or NULL if
// the user cancelled.
//
//export nebulaPicked
func nebulaPicked(handle C.uintptr_t, path *C.char) {
	h := cgo.Handle(handle)
	done := h.Value().(chan string)
	h.Delete()
	var p string
	if path != nil {
		p = C.GoString(path)
	}
	done <- p // buffered; never blocks
}
