//go:build unix

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// shortTempDir returns a directory whose paths fit in a socket address.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ngui")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func versionProblemAt(t *testing.T, addr string, timeout time.Duration) *problem {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := api.NewClient(addr).Version(ctx)
	if err == nil {
		t.Fatal("Version succeeded")
	}
	return problemFor(err, problemContext{})
}

// TestRealTransportErrors dials real sockets, so the errors are exactly
// what the client produces, not hand-wrapped copies.
func TestRealTransportErrors(t *testing.T) {
	dir := shortTempDir(t)

	t.Run("missing", func(t *testing.T) {
		if p := versionProblemAt(t, filepath.Join(dir, "none.sock"), 5*time.Second); p.ID != probServiceNotRunning {
			t.Fatalf("missing socket = %s (%s)", p.ID, p.Details)
		}
	})

	t.Run("refused", func(t *testing.T) {
		addr := filepath.Join(dir, "stale.sock")
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: addr, Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		l.SetUnlinkOnClose(false) // leave the file behind, like a crash
		_ = l.Close()
		if p := versionProblemAt(t, addr, 5*time.Second); p.ID != probServiceNotRunning {
			t.Fatalf("stale socket = %s (%s)", p.ID, p.Details)
		}
	})

	t.Run("permission", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores the socket's mode")
		}
		addr := filepath.Join(dir, "locked.sock")
		l, err := net.Listen("unix", addr)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		if err := os.Chmod(addr, 0); err != nil {
			t.Fatal(err)
		}
		if p := versionProblemAt(t, addr, 5*time.Second); p.ID != probNoPermission {
			t.Fatalf("mode 0000 socket = %s (%s)", p.ID, p.Details)
		}
	})

	t.Run("silent", func(t *testing.T) {
		addr := filepath.Join(dir, "silent.sock")
		l, err := net.Listen("unix", addr)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		// Accept and never answer. The connections close with the test.
		var mu sync.Mutex
		var conns []net.Conn
		t.Cleanup(func() {
			_ = l.Close()
			mu.Lock()
			defer mu.Unlock()
			for _, c := range conns {
				_ = c.Close()
			}
		})
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				mu.Lock()
				conns = append(conns, c)
				mu.Unlock()
			}
		}()
		if p := versionProblemAt(t, addr, 200*time.Millisecond); p.ID != probServiceNotResponding {
			t.Fatalf("silent service = %s (%s)", p.ID, p.Details)
		}
	})

}
