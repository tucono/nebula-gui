// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestServiceArgs(t *testing.T) {
	if got, want := serviceArgs(""), []string{"run", "--socket-group", "nebula-gui"}; !reflect.DeepEqual(got, want) {
		t.Errorf("serviceArgs(\"\") = %q", got)
	}
	args := serviceArgs(`DESKTOP-1\alice`)
	cmd, rest, err := splitCommand(args)
	if err != nil || cmd != "run" {
		t.Fatalf("splitCommand(%q) = %q, %v", args, cmd, err)
	}
	o, err := parseOptions(cmd, rest)
	if err != nil {
		t.Fatal(err)
	}
	if o.group != socketGroup || o.owner != `DESKTOP-1\alice` {
		t.Errorf("options = %+v", o)
	}
}

func TestParseInstall(t *testing.T) {
	if f, err := parseInstall(nil); err != nil || f != (installFlags{}) {
		t.Errorf("no flags: %+v, %v", f, err)
	}
	if f, err := parseInstall([]string{"--allow-user", "alice"}); err != nil || f.allowUser != "alice" || f.sessionUser {
		t.Errorf("--allow-user alice: %+v, %v", f, err)
	}
	if f, err := parseInstall([]string{"--allow-session-user"}); err != nil || !f.sessionUser || f.allowUser != "" {
		t.Errorf("--allow-session-user: %+v, %v", f, err)
	}
	if _, err := parseInstall([]string{"--allow-user", "alice", "--allow-session-user"}); err == nil {
		t.Error("both flags were accepted")
	}
	if _, err := parseInstall([]string{"alice"}); err == nil {
		t.Error("a bare user name was accepted")
	}
	if _, err := parseInstall([]string{"--socket-group", "x"}); err == nil {
		t.Error("an unknown flag was accepted")
	}
}

// TestSessionUser runs the session query for real. A CI runner may run
// in a session no one is signed in to, which is the other valid answer.
func TestSessionUser(t *testing.T) {
	u, err := sessionUser()
	switch {
	case errors.Is(err, errNoSessionUser):
		t.Log("no one signed in to this session")
	case err != nil:
		t.Fatal(err)
	case u == "" || strings.HasSuffix(u, `\`):
		t.Errorf("sessionUser() = %q", u)
	default:
		t.Logf("session user %q", u)
	}
}

// fakeEventLog records Event Log entries.
type fakeEventLog struct {
	mu      sync.Mutex
	entries []string
}

func (l *fakeEventLog) add(level, msg string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, level+": "+msg)
	return nil
}
func (l *fakeEventLog) Info(_ uint32, msg string) error  { return l.add("info", msg) }
func (l *fakeEventLog) Error(_ uint32, msg string) error { return l.add("error", msg) }
func (l *fakeEventLog) Close() error                     { return nil }

func (l *fakeEventLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.entries, "\n")
}

// execute runs h.Execute in the background, as svc.Run does.
func execute(h *handler) (r chan svc.ChangeRequest, s chan svc.Status, result chan [2]any) {
	r = make(chan svc.ChangeRequest)
	s = make(chan svc.Status, 10)
	result = make(chan [2]any, 1)
	go func() {
		ok, code := h.Execute(nil, r, s)
		result <- [2]any{ok, code}
	}()
	return r, s, result
}

func waitStatus(t *testing.T, s <-chan svc.Status, want svc.State) svc.Status {
	t.Helper()
	select {
	case st := <-s:
		if st.State != want {
			t.Fatalf("status = %d, want %d", st.State, want)
		}
		return st
	case <-time.After(5 * time.Second):
		t.Fatalf("no status %d", want)
	}
	return svc.Status{}
}

func waitResult(t *testing.T, result <-chan [2]any) (bool, uint32) {
	t.Helper()
	select {
	case r := <-result:
		return r[0].(bool), r[1].(uint32)
	case <-time.After(5 * time.Second):
		t.Fatal("Execute didn't return")
	}
	return false, 0
}

func TestHandlerStop(t *testing.T) {
	for _, cmd := range []svc.Cmd{svc.Stop, svc.Shutdown} {
		elog := &fakeEventLog{}
		stopped := make(chan struct{})
		h := &handler{elog: elog, serve: func(ctx context.Context) error {
			<-ctx.Done()
			close(stopped) // Nebula stopped before Execute returns
			return nil
		}}
		r, s, result := execute(h)
		waitStatus(t, s, svc.StartPending)
		st := waitStatus(t, s, svc.Running)
		if st.Accepts != svc.AcceptStop|svc.AcceptShutdown {
			t.Errorf("accepts = %d", st.Accepts)
		}

		// Interrogate gets the current status back and changes nothing.
		r <- svc.ChangeRequest{Cmd: svc.Interrogate, CurrentStatus: st}
		waitStatus(t, s, svc.Running)

		r <- svc.ChangeRequest{Cmd: cmd}
		waitStatus(t, s, svc.StopPending)
		ok, code := waitResult(t, result)
		select {
		case <-stopped:
		default:
			t.Fatal("Execute returned before serve did")
		}
		if ok || code != 0 {
			t.Errorf("cmd %d: Execute = %v, %d; want false, 0", cmd, ok, code)
		}
		if got := elog.String(); !strings.Contains(got, "info: nebula-guid started") || !strings.HasSuffix(got, "info: nebula-guid stopped") {
			t.Errorf("event log:\n%s", got)
		}
	}
}

func TestHandlerStopError(t *testing.T) {
	elog := &fakeEventLog{}
	h := &handler{elog: elog, serve: func(ctx context.Context) error {
		<-ctx.Done()
		return errors.New("http: shutdown timed out")
	}}
	r, s, result := execute(h)
	waitStatus(t, s, svc.StartPending)
	waitStatus(t, s, svc.Running)
	r <- svc.ChangeRequest{Cmd: svc.Stop}
	// A requested stop never triggers the restart policy.
	if ok, code := waitResult(t, result); ok || code != 0 {
		t.Errorf("Execute = %v, %d; want false, 0", ok, code)
	}
	if got := elog.String(); !strings.Contains(got, "error: nebula-guid stopped with an error: http: shutdown timed out") {
		t.Errorf("event log:\n%s", got)
	}
}

func TestHandlerServeFails(t *testing.T) {
	for name, serveErr := range map[string]error{
		"error":  errors.New("listen on pipe: access denied"),
		"no err": nil,
	} {
		t.Run(name, func(t *testing.T) {
			elog := &fakeEventLog{}
			h := &handler{elog: elog, serve: func(context.Context) error { return serveErr }}
			_, s, result := execute(h)
			waitStatus(t, s, svc.StartPending)
			if ok, code := waitResult(t, result); !ok || code != exitServeFailed {
				t.Errorf("Execute = %v, %d; want true, %d", ok, code, exitServeFailed)
			}
			if got := elog.String(); !strings.Contains(got, "error: nebula-guid failed: ") {
				t.Errorf("event log:\n%s", got)
			}
		})
	}
}
