// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"slices"
	"strconv"
)

// The installed service's names, as the packages set them up (see
// "Installing the service" in docs/m4-plan.md). The problems quote them in
// commands on every OS, so they're defined everywhere.
const (
	linuxServiceUnit   = "nebula-guid"
	darwinServiceLabel = "io.github.tucono.nebula-guid"
	windowsService     = "nebula-guid"
	// accessGroup is the group allowed to use the service.
	accessGroup = "nebula-gui"
)

// serviceSetup is what the app can tell about the installed service while
// it can't reach it, so the problem shown can say "not installed" or offer
// to start it. probeSetup in setup_<os>.go fills it in.
type serviceSetup struct {
	// Known is set when the app could tell whether the service is
	// installed. When it isn't, the problems keep to general advice.
	Known     bool
	Installed bool
	// CanStart is set when the app can ask the OS to start the service,
	// with the OS asking for the password.
	CanStart bool
	// GroupPending is set when this user has been added to the access
	// group since signing in (Linux).
	GroupPending bool
}

// errStartCancelled wraps the error from startService when the user said
// no when the OS asked, or the OS didn't allow it.
var errStartCancelled = errors.New("not allowed")

// groupPendingIn reports whether the group gid is one of the user's groups
// (members, as the system's group list now has it) but not one of this
// session's (session, fixed at sign-in).
func groupPendingIn(gid string, members []string, session []int) bool {
	if !slices.Contains(members, gid) {
		return false
	}
	for _, id := range session {
		if strconv.Itoa(id) == gid {
			return false
		}
	}
	return true
}
