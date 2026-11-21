// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

// remove-everything is the Linux uninstall's "remove everything", for
// rpm, which has no purge: run it, then remove the package. A .deb's
// purge does the same from postremove.sh.

func parseRemoveEverything(args []string) (yes bool, err error) {
	fs := flag.NewFlagSet("remove-everything", flag.ContinueOnError)
	fs.BoolVar(&yes, "yes", false, "don't ask first")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	if fs.NArg() > 0 {
		return false, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return yes, nil
}

// everything is what remove-everything does, step by step, so tests can
// replace each step.
type everything struct {
	stateDir string
	// running reports whether the service is running.
	running func() bool
	users   func() ([]userfiles.User, error)
	remove  func(u userfiles.User, autostartOnly bool) ([]string, error)
	// removeAll removes the state directory.
	removeAll func(string) error
	// deleteAccounts removes the service's user and the access group.
	deleteAccounts func() error
	in             io.Reader
	out, errOut    io.Writer
}

// errNotConfirmed is returned when the answer to the question isn't yes.
var errNotConfirmed = errors.New("nothing was removed")

func (e everything) run(yes bool) error {
	if e.running() {
		return errors.New("the service is running; stop it first: sudo systemctl disable --now nebula-guid")
	}
	if !yes {
		_, _ = fmt.Fprintf(e.out, "This removes everything nebula-gui saved on this computer: the imported\n"+
			"config and the service's logs in %s, every user's saved peer\n"+
			"names, app log and settings, and the nebula-guid user and nebula-gui group.\n"+
			"Remove everything? [y/N] ", e.stateDir)
		answer, _ := bufio.NewReader(e.in).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
		default:
			return errNotConfirmed
		}
	}

	failed := false
	users, err := e.users()
	if err != nil {
		failed = true
		_, _ = fmt.Fprintln(e.errOut, "nebula-guid: listing users:", err)
	}
	if err := removeFor(users, false, e.remove, e.out, e.errOut); err != nil {
		failed = true
	}
	if err := e.removeAll(e.stateDir); err != nil {
		failed = true
		_, _ = fmt.Fprintln(e.errOut, "nebula-guid:", err)
	} else {
		_, _ = fmt.Fprintln(e.out, "removed", e.stateDir)
	}
	if err := e.deleteAccounts(); err != nil {
		failed = true
		_, _ = fmt.Fprintln(e.errOut, "nebula-guid:", err)
	}
	if failed {
		return errSomeNotRemoved
	}
	_, _ = fmt.Fprintln(e.out, "Everything is removed. Now remove the package: sudo dnf remove nebula-gui\n"+
		"(or sudo apt purge nebula-gui).")
	return nil
}
