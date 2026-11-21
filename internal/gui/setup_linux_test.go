//go:build linux

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import "testing"

func TestLinuxSetup(t *testing.T) {
	for _, tc := range []struct {
		installed, systemd bool
		want               serviceSetup
	}{
		{false, true, serviceSetup{Known: true}},
		{true, false, serviceSetup{Known: true, Installed: true}},
		{true, true, serviceSetup{Known: true, Installed: true, CanStart: true}},
	} {
		if got := linuxSetup(tc.installed, tc.systemd, false); got != tc.want {
			t.Errorf("linuxSetup(%v, %v) = %+v, want %+v", tc.installed, tc.systemd, got, tc.want)
		}
	}
	if !linuxSetup(false, false, true).GroupPending {
		t.Error("GroupPending dropped")
	}
}

// TestProbeSetup runs the real probe; what it finds depends on the
// machine, so it only checks that it can tell.
func TestProbeSetup(t *testing.T) {
	if s := probeSetup(); !s.Known {
		t.Fatalf("probeSetup() = %+v", s)
	}
}
