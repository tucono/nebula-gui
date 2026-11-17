//go:build windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import "testing"

func TestWindowsSetup(t *testing.T) {
	if s := windowsSetup("nebula-guid-test-missing"); s != (serviceSetup{Known: true}) {
		t.Errorf("missing service: %+v", s)
	}
	// The Event Log service is on every Windows.
	if s := windowsSetup("EventLog"); s != (serviceSetup{Known: true, Installed: true, CanStart: true}) {
		t.Errorf("EventLog: %+v", s)
	}
}
