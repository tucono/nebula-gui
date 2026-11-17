// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

var pipeSeq atomic.Int32

// testInstanceAddr is a pipe name no other test or copy uses.
func testInstanceAddr(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`\\.\pipe\nebula-gui-test-%d-%d`, os.Getpid(), pipeSeq.Add(1))
}

func TestInstanceSDDLIsUserOnly(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	got := instanceSDDL(sid)
	if want := "D:P(A;;GA;;;" + sid + ")"; got != want {
		t.Errorf("instanceSDDL = %q, want %q", got, want)
	}
	for _, other := range []string{";SY)", ";BA)", ";WD)"} {
		if strings.Contains(got, other) {
			t.Errorf("instanceSDDL %q grants %s", got, other)
		}
	}
}

func TestDefaultInstanceAddr(t *testing.T) {
	sid, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	got, err := defaultInstanceAddr()
	if err != nil {
		t.Fatal(err)
	}
	if want := `\\.\pipe\nebula-gui-` + sid; got != want {
		t.Errorf("defaultInstanceAddr() = %q, want %q", got, want)
	}
}

func TestInstanceSecondListenFails(t *testing.T) {
	addr := testInstanceAddr(t)
	runningInstance(t, addr)
	if l, err := listenInstance(addr); err == nil {
		_ = l.Close()
		t.Fatal("a second listener took the same pipe name")
	}
}
