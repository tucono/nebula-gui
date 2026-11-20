// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

// removeUserFilesFlags are the remove-user-files command's flags.
type removeUserFilesFlags struct {
	autostartOnly bool
}

func parseRemoveUserFiles(args []string) (removeUserFilesFlags, error) {
	var f removeUserFilesFlags
	fs := flag.NewFlagSet("remove-user-files", flag.ContinueOnError)
	fs.BoolVar(&f.autostartOnly, "autostart-only", false,
		"remove only each user's start-at-sign-in entry, keeping their settings (a normal uninstall)")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return f, nil
}

// errSomeNotRemoved is returned when some files couldn't be removed; each
// has been printed already.
var errSomeNotRemoved = errors.New("some files couldn't be removed")

// removeUserFiles removes every user's nebula-gui files, for the
// uninstallers. It needs root or administrator rights to reach other
// people's home folders.
func removeUserFiles(args []string) error {
	f, err := parseRemoveUserFiles(args)
	if err != nil {
		return err
	}
	if !isAdmin() {
		return errors.New("remove-user-files must run as root or administrator")
	}
	users, err := userfiles.Users()
	if err != nil {
		return err
	}
	return removeFor(users, f.autostartOnly, userfiles.RemoveUser, os.Stdout, os.Stderr)
}

// removeFor removes each user's files with remove, printing what it
// removed to out and what it couldn't to errOut. One user's failure
// doesn't stop the others.
func removeFor(users []userfiles.User, autostartOnly bool,
	remove func(u userfiles.User, autostartOnly bool) ([]string, error), out, errOut io.Writer,
) error {
	failed := false
	for _, u := range users {
		removed, err := remove(u, autostartOnly)
		for _, p := range removed {
			_, _ = fmt.Fprintln(out, "removed", p)
		}
		if err != nil {
			failed = true
			_, _ = fmt.Fprintf(errOut, "nebula-guid: %s: %v\n", u.Name, err)
		}
	}
	if failed {
		return errSomeNotRemoved
	}
	return nil
}
