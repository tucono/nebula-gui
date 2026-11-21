// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestPostremoveNamesLinuxFiles checks that the .deb's purge, which is
// shell because the binaries are gone by then, removes the files For
// names on Linux, for the accounts Users lists.
func TestPostremoveNamesLinuxFiles(t *testing.T) {
	b, err := os.ReadFile("../../packaging/linux/postremove.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	m := regexp.MustCompile(`(?s)\nuser_paths="([^"]*)"`).FindStringSubmatch(s)
	if m == nil {
		t.Fatal(`postremove.sh has no user_paths="…"`)
	}
	f := relFor("linux")
	want := append(f.Dirs, f.Autostart)
	if got := strings.Split(m[1], "\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("postremove.sh user_paths = %q, want %q", got, want)
	}
	for _, rule := range []string{
		`"$uid" -lt ` + strconv.Itoa(firstUID),
		`"$uid" -eq ` + strconv.Itoa(nobodyUID),
	} {
		if !strings.Contains(s, rule) {
			t.Errorf("postremove.sh doesn't skip accounts with %s, as parsePasswd does", rule)
		}
	}
}
