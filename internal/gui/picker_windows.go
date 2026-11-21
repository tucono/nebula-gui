//go:build windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// platformOpen is File Explorer's Open and Save As dialogs, owned by the
// app's window.
var platformOpen nativeOpenFunc = explorerOpenFile

// winWindowPrefix starts a window handle that names an HWND; see
// windowHandle.
const winWindowPrefix = "hwnd:"

var (
	comdlg32                 = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW     = comdlg32.NewProc("GetSaveFileNameW")
	procCommDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")

	user32                = windows.NewLazySystemDLL("user32.dll")
	procEnumThreadWindows = user32.NewProc("EnumThreadWindows")
	procPostMessageW      = user32.NewProc("PostMessageW")
)

// openFileName is OPENFILENAMEW.
type openFileName struct {
	structSize    uint32
	owner         uintptr
	instance      uintptr
	filter        *uint16
	customFilter  *uint16
	maxCustFilter uint32
	filterIndex   uint32
	file          *uint16
	maxFile       uint32
	fileTitle     *uint16
	maxFileTitle  uint32
	initialDir    *uint16
	title         *uint16
	flags         uint32
	fileOffset    uint16
	fileExtension uint16
	defExt        *uint16
	custData      uintptr
	fnHook        uintptr
	templateName  *uint16
	pvReserved    uintptr
	dwReserved    uint32
	flagsEx       uint32
}

const (
	ofnOverwritePrompt = 0x00000002
	ofnNoChangeDir     = 0x00000008
	ofnPathMustExist   = 0x00000800
	ofnFileMustExist   = 0x00001000
	ofnExplorer        = 0x00080000

	wmClose = 0x0010

	// maxPath is the longest path the dialog may return, in UTF-16 units.
	maxPath = 32768
)

// explorerOpenFile shows the Open dialog, or with req.Save set the Save As
// dialog, and waits for the user to close it. parent names the window that
// owns it; the dialog keeps that window disabled while it's open.
func explorerOpenFile(ctx context.Context, parent string, req openRequest) (string, error) {
	if err := comdlg32.Load(); err != nil {
		return "", &errNoNativePicker{Err: err}
	}
	var owner uintptr
	if h, ok := strings.CutPrefix(parent, winWindowPrefix); ok {
		v, _ := strconv.ParseUint(h, 16, 64)
		owner = uintptr(v)
	}

	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	tid := make(chan uint32, 1)
	go func() {
		// The dialog runs a message loop on this thread, and needs COM
		// initialized on it.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
			defer windows.CoUninitialize()
		}
		tid <- windows.GetCurrentThreadId()
		path, err := runFileDialog(owner, req)
		done <- result{path, err}
	}()
	thread := <-tid

	select {
	case r := <-done:
		return r.path, r.err
	case <-ctx.Done():
		closeThreadWindows(thread)
		// Give the dialog a moment to close, so it doesn't outlive the app.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return "", ctx.Err()
	}
}

// runFileDialog shows the dialog on the calling thread. It returns "" if
// the user cancelled.
func runFileDialog(owner uintptr, req openRequest) (string, error) {
	buf := make([]uint16, maxPath)
	if req.Save && req.FileName != "" {
		copy(buf[:maxPath-1], windows.StringToUTF16(req.FileName))
	}
	filter := utf16Multi(windowsFilter(req.FilterName, req.Extensions))
	ofn := openFileName{
		owner:       owner,
		filter:      &filter[0],
		filterIndex: 1,
		file:        &buf[0],
		maxFile:     maxPath,
		flags:       ofnExplorer | ofnNoChangeDir | ofnPathMustExist,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))
	if req.Title != "" {
		ofn.title = windows.StringToUTF16Ptr(req.Title)
	}
	if req.StartDir != "" {
		ofn.initialDir = windows.StringToUTF16Ptr(req.StartDir)
	}
	proc := procGetOpenFileNameW
	if req.Save {
		proc = procGetSaveFileNameW
		ofn.flags |= ofnOverwritePrompt
		// Adds the extension when the user types a name without one.
		if len(req.Extensions) > 0 {
			ofn.defExt = windows.StringToUTF16Ptr(strings.TrimPrefix(req.Extensions[0], "."))
		}
	} else {
		ofn.flags |= ofnFileMustExist
	}

	// The struct is passed only for the call, the documented use of
	// unsafe.Pointer with syscalls.
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn))) //nolint:gosec // see above
	runtime.KeepAlive(filter)
	runtime.KeepAlive(buf)
	if ok != 0 {
		return windows.UTF16ToString(buf), nil
	}
	// Zero with no extended error is a cancel.
	if code, _, _ := procCommDlgExtendedError.Call(); code != 0 {
		return "", fmt.Errorf("file dialog failed: error 0x%x", code)
	}
	return "", nil
}

// windowsFilter lists the dialog's file types as name/pattern pairs: the
// requested extensions first, then all files. An empty name means "Config
// files".
func windowsFilter(name string, exts []string) []string {
	var globs []string
	for _, e := range exts {
		if e = strings.TrimPrefix(e, "."); e != "" {
			globs = append(globs, "*."+e)
		}
	}
	all := []string{"All files (*.*)", "*.*"}
	if len(globs) == 0 {
		return all
	}
	if name == "" {
		name = "Config files"
	}
	pattern := strings.Join(globs, ";")
	return append([]string{name + " (" + pattern + ")", pattern}, all...)
}

// utf16Multi encodes strings as the dialog's filter wants them: each ends
// in a NUL, and the list ends in another.
func utf16Multi(parts []string) []uint16 {
	var out []uint16
	for _, p := range parts {
		out = append(out, windows.StringToUTF16(p)...)
	}
	return append(out, 0)
}

var (
	closeOnce sync.Once
	closeCB   uintptr
)

// closeThreadWindows asks every top-level window of thread to close,
// which cancels a file dialog running there.
func closeThreadWindows(thread uint32) {
	closeOnce.Do(func() {
		closeCB = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
			_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
			return 1 // continue
		})
	})
	_, _, _ = procEnumThreadWindows.Call(uintptr(thread), closeCB, 0)
}
