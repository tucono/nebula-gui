// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/ipc"
)

// runningInstance claims addr and serves it, counting show requests on the
// returned channel.
func runningInstance(t *testing.T, addr string) (*instance, <-chan struct{}) {
	t.Helper()
	in, err := claimInstance(addr, true)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	t.Cleanup(func() { _ = in.Close() })
	shown := make(chan struct{}, 4)
	go in.serve(func() { shown <- struct{}{} })
	return in, shown
}

func TestInstanceSecondCopyShowsFirst(t *testing.T) {
	addr := testInstanceAddr(t)
	_, shown := runningInstance(t, addr)

	if _, err := claimInstance(addr, true); !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("second claim = %v, want errAlreadyRunning", err)
	}
	select {
	case <-shown:
	case <-time.After(5 * time.Second):
		t.Fatal("the first copy wasn't asked to show its window")
	}
}

func TestInstanceHiddenSecondCopyLeavesFirstAlone(t *testing.T) {
	addr := testInstanceAddr(t)
	_, shown := runningInstance(t, addr)

	if _, err := claimInstance(addr, false); !errors.Is(err, errAlreadyRunning) {
		t.Fatalf("second claim = %v, want errAlreadyRunning", err)
	}
	select {
	case <-shown:
		t.Fatal("a copy started hidden showed the first copy's window")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestInstanceIgnoresOtherMessages(t *testing.T) {
	addr := testInstanceAddr(t)
	_, shown := runningInstance(t, addr)

	for _, msg := range []string{"quit\n", "show now\n", "showshowshowshowshowshow\n", "show"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		c, err := ipc.Dial(ctx, addr)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write([]byte(msg)); err != nil {
			// The copy may hang up before the write finishes, as on
			// Windows for a message that's too long. That's dropping it.
			_ = c.Close()
			continue
		}
		if msg == "show" {
			// No newline: the copy waits for the rest, then gives up.
			_ = c.SetReadDeadline(time.Now().Add(instanceWait + time.Second))
		}
		if b, _ := io.ReadAll(c); len(b) != 0 {
			t.Errorf("%q got a reply %q", msg, b)
		}
		_ = c.Close()
	}
	select {
	case <-shown:
		t.Fatal("a message other than show showed the window")
	default:
	}
}

func TestInstanceCloseFreesAddress(t *testing.T) {
	addr := testInstanceAddr(t)
	in, _ := runningInstance(t, addr)
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := claimInstance(addr, true)
	if err != nil {
		t.Fatalf("claim after Close: %v", err)
	}
	_ = next.Close()
}

func TestInstanceServeStopsOnClose(t *testing.T) {
	in, err := claimInstance(testInstanceAddr(t), true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		in.serve(func() {})
		close(done)
	}()
	_ = in.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("serve didn't return after Close")
	}
}
