// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/tucono/nebula-gui/internal/ipc"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		args []string
		cmd  string
		rest []string
	}{
		{nil, "", nil},
		{[]string{"--listen", "x"}, "", []string{"--listen", "x"}},
		{[]string{"-version"}, "", []string{"-version"}},
		{[]string{"install", "--allow-user", "alice"}, "install", []string{"--allow-user", "alice"}},
		{[]string{"uninstall"}, "uninstall", []string{}},
		{[]string{"run", "--socket-group", "nebula-gui"}, "run", []string{"--socket-group", "nebula-gui"}},
		{[]string{"remove-user-files", "--autostart-only"}, "remove-user-files", []string{"--autostart-only"}},
	}
	for _, tc := range tests {
		cmd, rest, err := splitCommand(tc.args)
		if err != nil || cmd != tc.cmd || !reflect.DeepEqual(rest, tc.rest) {
			t.Errorf("splitCommand(%q) = %q, %q, %v; want %q, %q", tc.args, cmd, rest, err, tc.cmd, tc.rest)
		}
	}
	if _, _, err := splitCommand([]string{"start"}); err == nil || !strings.Contains(err.Error(), `"start"`) {
		t.Errorf("unknown command: err = %v", err)
	}
}

func TestParseOptions(t *testing.T) {
	o, err := parseOptions("run", nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.listen != ipc.DefaultAddress() || o.owner != "" || o.group != "" || o.version {
		t.Errorf("defaults = %+v", o)
	}

	o, err = parseOptions("run", []string{"--socket-group", "nebula-gui", "--socket-owner", "alice", "--state-dir", "/tmp/s", "--listen", "/tmp/sock"})
	if err != nil {
		t.Fatal(err)
	}
	want := options{listen: "/tmp/sock", owner: "alice", group: "nebula-gui", stateDir: "/tmp/s"}
	if *o != want {
		t.Errorf("options = %+v, want %+v", *o, want)
	}

	if _, err := parseOptions("run", []string{"--socket-group", "g", "extra"}); err == nil || !strings.Contains(err.Error(), `"extra"`) {
		t.Errorf("extra argument: err = %v", err)
	}
	if _, err := parseOptions("run", []string{"--no-such-flag"}); err == nil {
		t.Error("unknown flag accepted")
	}
	if _, err := parseOptions("run", []string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: err = %v, want flag.ErrHelp", err)
	}
}

func TestReleaseHasNoDevFlag(t *testing.T) {
	if devBuild {
		t.Skip("dev build")
	}
	if _, err := parseOptions("run", []string{"--dev"}); err == nil {
		t.Fatal("--dev accepted by a release build")
	}
}
