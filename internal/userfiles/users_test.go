// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParsePasswd(t *testing.T) {
	in := strings.Join([]string{
		"root:x:0:0:root:/root:/bin/bash",
		"nebula-guid:x:998:998:nebula-gui background service:/:/usr/sbin/nologin",
		"alice:x:1000:1000:Alice,,,:/home/alice:/bin/bash",
		"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin",
		"bob:x:1001:1001::/home/bob:/bin/zsh",
		"dev:x:1002:1002::relative:/bin/sh",
		"weird:x:1003:1003::/:/bin/sh",
		"broken:x:notanumber:1:::",
		"short:x:1004",
		"",
	}, "\n")
	got := parsePasswd(strings.NewReader(in))
	want := []User{{"alice", "/home/alice"}, {"bob", "/home/bob"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePasswd = %+v, want %+v", got, want)
	}
}

func TestHomesIn(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"alice", "bob", "Shared", ".localized-dir"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(dir, ".localized"))
	got, err := homesIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []User{{"alice", filepath.Join(dir, "alice")}, {"bob", filepath.Join(dir, "bob")}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("homesIn = %+v, want %+v", got, want)
	}
}
