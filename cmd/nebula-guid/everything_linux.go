// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/tucono/nebula-gui/internal/service"
	"github.com/tucono/nebula-gui/internal/userfiles"
)

// removeEverything removes the service's state, every user's files, and
// the service's user and group, after asking unless --yes.
func removeEverything(args []string) error {
	yes, err := parseRemoveEverything(args)
	if err != nil {
		return err
	}
	if !isAdmin() {
		return errors.New("remove-everything must run as root")
	}
	return everything{
		stateDir:       service.DefaultStateDir(),
		running:        serviceRunning,
		users:          userfiles.Users,
		remove:         userfiles.RemoveUser,
		removeAll:      os.RemoveAll,
		deleteAccounts: deleteAccounts,
		in:             os.Stdin,
		out:            os.Stdout,
		errOut:         os.Stderr,
	}.run(yes)
}

// serviceRunning asks systemd. Without systemd (a container) nothing runs
// it.
func serviceRunning() bool {
	return exec.Command("systemctl", "is-active", "--quiet", "nebula-guid.service").Run() == nil
}

// deleteAccounts removes the nebula-guid user, then the nebula-gui group
// and the user's own group if userdel left it. One that isn't there is
// skipped.
func deleteAccounts() error {
	var errs []error
	if exec.Command("getent", "passwd", "nebula-guid").Run() == nil {
		if out, err := exec.Command("userdel", "nebula-guid").CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("userdel nebula-guid: %w: %s", err, out))
		}
	}
	if exec.Command("getent", "group", "nebula-gui").Run() == nil {
		if out, err := exec.Command("groupdel", "nebula-gui").CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("groupdel nebula-gui: %w: %s", err, out))
		}
	}
	if exec.Command("getent", "group", "nebula-guid").Run() == nil {
		if out, err := exec.Command("groupdel", "nebula-guid").CombinedOutput(); err != nil {
			errs = append(errs, fmt.Errorf("groupdel nebula-guid: %w: %s", err, out))
		}
	}
	return errors.Join(errs...)
}
