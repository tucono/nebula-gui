//go:build linux

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

const (
	// linuxServiceExe is where the package installs the service.
	linuxServiceExe = "/usr/libexec/nebula-gui/nebula-guid"
	// systemdRunDir exists while systemd is the init system.
	systemdRunDir = "/run/systemd/system"
)

func probeSetup() serviceSetup {
	return linuxSetup(exists(linuxServiceExe), exists(systemdRunDir), groupPending())
}

func linuxSetup(installed, systemd, pending bool) serviceSetup {
	return serviceSetup{Known: true, Installed: installed, CanStart: installed && systemd, GroupPending: pending}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// groupPending reports whether the user is in the access group according
// to the system's group list but not in this session.
func groupPending() bool {
	g, err := user.LookupGroup(accessGroup)
	if err != nil {
		return false
	}
	u, err := user.Current()
	if err != nil {
		return false
	}
	members, err := u.GroupIds()
	if err != nil {
		return false
	}
	session, err := os.Getgroups()
	if err != nil {
		return false
	}
	return groupPendingIn(g.Gid, members, append(session, os.Getegid()))
}

// startService asks systemd to start the service. systemd asks polkit,
// which has the desktop ask for an administrator's password; nothing else
// runs as root.
func startService(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "systemctl", "start", linuxServiceUnit).CombinedOutput() //nolint:gosec // fixed arguments
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	if strings.Contains(msg, "Access denied") {
		return fmt.Errorf("%w: %s", errStartCancelled, msg)
	}
	return fmt.Errorf("systemctl start %s: %s", linuxServiceUnit, msg)
}
