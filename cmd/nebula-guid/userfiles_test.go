// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

func TestParseRemoveUserFiles(t *testing.T) {
	if f, err := parseRemoveUserFiles(nil); err != nil || f.autostartOnly {
		t.Errorf("no flags = %+v, %v", f, err)
	}
	if f, err := parseRemoveUserFiles([]string{"--autostart-only"}); err != nil || !f.autostartOnly {
		t.Errorf("--autostart-only = %+v, %v", f, err)
	}
	if _, err := parseRemoveUserFiles([]string{"alice"}); err == nil {
		t.Error("an argument was accepted")
	}
	if _, err := parseRemoveUserFiles([]string{"--everything"}); err == nil {
		t.Error("an unknown flag was accepted")
	}
}

func TestRemoveForCarriesOn(t *testing.T) {
	users := []userfiles.User{{Name: "alice", Home: "/home/alice"}, {Name: "bob", Home: "/home/bob"}}
	var seen []string
	remove := func(home string, autostartOnly bool) ([]string, error) {
		if !autostartOnly {
			t.Error("autostartOnly wasn't passed on")
		}
		seen = append(seen, home)
		if home == "/home/alice" {
			return []string{"/home/alice/a"}, errors.New("permission denied")
		}
		return []string{"/home/bob/b"}, nil
	}
	var out, errOut strings.Builder
	err := removeFor(users, true, remove, &out, &errOut)
	if !errors.Is(err, errSomeNotRemoved) {
		t.Errorf("err = %v, want errSomeNotRemoved", err)
	}
	if len(seen) != 2 {
		t.Errorf("tried %q, want both users", seen)
	}
	if out.String() != "removed /home/alice/a\nremoved /home/bob/b\n" {
		t.Errorf("out = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "alice: permission denied") {
		t.Errorf("errOut = %q", errOut.String())
	}
}
