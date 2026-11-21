//go:build darwin

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

/*
#cgo LDFLAGS: -framework Cocoa
void nebulaWatchReopen(void);
*/
import "C"

import "sync/atomic"

// macOS doesn't start a second copy when the running app is opened again
// from Spotlight, Finder or the Dock: it sends the running one a reopen
// event, which neither Fyne nor GLFW handles. reopen_darwin.m catches it.

var onReopen atomic.Pointer[func()]

// watchReopen calls f, on the main thread, each time the app is opened
// again. Call it once the app has started: AppKit installs its own handler
// while launching, and the later one wins.
func watchReopen(f func()) {
	onReopen.Store(&f)
	C.nebulaWatchReopen()
}

//export nebulaReopened
func nebulaReopened() {
	if f := onReopen.Load(); f != nil {
		(*f)()
	}
}
