// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

func TestParseRemoveEverything(t *testing.T) {
	if yes, err := parseRemoveEverything(nil); err != nil || yes {
		t.Errorf("no flags = %v, %v", yes, err)
	}
	if yes, err := parseRemoveEverything([]string{"--yes"}); err != nil || !yes {
		t.Errorf("--yes = %v, %v", yes, err)
	}
	if _, err := parseRemoveEverything([]string{"now"}); err == nil {
		t.Error("an argument was accepted")
	}
}

// fakeEverything records the steps it takes.
func fakeEverything(running bool, answer string) (*everything, *[]string, *strings.Builder) {
	var steps []string
	var out strings.Builder
	e := &everything{
		stateDir: "/var/lib/nebula-gui",
		running:  func() bool { return running },
		users: func() ([]userfiles.User, error) {
			return []userfiles.User{{Name: "alice", Home: "/home/alice"}}, nil
		},
		remove: func(u userfiles.User, autostartOnly bool) ([]string, error) {
			if autostartOnly {
				steps = append(steps, "autostart only")
			}
			steps = append(steps, "user "+u.Name)
			return nil, nil
		},
		removeAll:      func(p string) error { steps = append(steps, "state "+p); return nil },
		deleteAccounts: func() error { steps = append(steps, "accounts"); return nil },
		in:             strings.NewReader(answer),
		out:            &out,
		errOut:         &out,
	}
	return e, &steps, &out
}

func TestRemoveEverything(t *testing.T) {
	all := []string{"user alice", "state /var/lib/nebula-gui", "accounts"}
	for _, tc := range []struct {
		name    string
		running bool
		yes     bool
		answer  string
		want    []string
		err     bool
	}{
		{"yes typed", false, false, "y\n", all, false},
		{"YES typed", false, false, " YES \n", all, false},
		{"--yes", false, true, "", all, false},
		{"no", false, false, "n\n", nil, true},
		{"enter", false, false, "\n", nil, true},
		{"end of input", false, false, "", nil, true},
		{"service running", true, true, "", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, steps, out := fakeEverything(tc.running, tc.answer)
			err := e.run(tc.yes)
			if (err != nil) != tc.err {
				t.Errorf("err = %v", err)
			}
			if !reflect.DeepEqual(*steps, tc.want) {
				t.Errorf("steps = %q, want %q", *steps, tc.want)
			}
			if tc.yes == strings.Contains(out.String(), "[y/N]") && !tc.running {
				t.Errorf("asked = %v with --yes = %v", !tc.yes, tc.yes)
			}
		})
	}
}

func TestRemoveEverythingCarriesOn(t *testing.T) {
	e, steps, out := fakeEverything(false, "")
	e.users = func() ([]userfiles.User, error) { return nil, errors.New("no getent") }
	e.removeAll = func(string) error { return errors.New("busy") }
	if err := e.run(true); !errors.Is(err, errSomeNotRemoved) {
		t.Errorf("err = %v, want errSomeNotRemoved", err)
	}
	if !reflect.DeepEqual(*steps, []string{"accounts"}) {
		t.Errorf("steps = %q: the accounts should still go", *steps)
	}
	if !strings.Contains(out.String(), "no getent") || !strings.Contains(out.String(), "busy") {
		t.Errorf("out = %q", out.String())
	}
}
