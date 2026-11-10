// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix

package ipc

import (
	"context"
	"errors"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"
)

// shortDir returns a temporary directory with a short path. t.TempDir paths
// can exceed the 104-byte socket path limit on macOS.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ipc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func echoOnce(t *testing.T, l net.Listener, addr string) {
	t.Helper()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		buf := make([]byte, 4)
		n, _ := c.Read(buf)
		_, _ = c.Write(buf[:n])
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := c.Read(buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}

func TestListenDialAndCleanup(t *testing.T) {
	addr := filepath.Join(shortDir(t), "sub", "s.sock")
	l, err := Listen(addr, ListenOptions{})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	fi, err := os.Lstat(addr)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
	if got := l.Addr().String(); got != addr {
		t.Errorf("Addr() = %q, want %q", got, addr)
	}
	entries, _ := os.ReadDir(filepath.Dir(addr))
	if len(entries) != 1 {
		t.Errorf("temporary directory left behind: %v", entries)
	}

	echoOnce(t, l, addr)

	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(addr); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket not removed on Close: %v", err)
	}
}

func TestListenGroupMode(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Skip(err)
	}
	addr := filepath.Join(shortDir(t), "s.sock")
	l, err := Listen(addr, ListenOptions{Group: g.Name})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	fi, err := os.Lstat(addr)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o660 {
		t.Errorf("mode = %o, want 660", got)
	}
}

func TestListenRemovesStaleSocket(t *testing.T) {
	addr := filepath.Join(shortDir(t), "s.sock")
	// A socket file with no listener, as left by a crashed service.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: addr, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	_ = stale.Close()

	l, err := Listen(addr, ListenOptions{})
	if err != nil {
		t.Fatalf("Listen over stale socket: %v", err)
	}
	defer func() { _ = l.Close() }()
	echoOnce(t, l, addr)
}

func TestListenRefusesLiveSocket(t *testing.T) {
	addr := filepath.Join(shortDir(t), "s.sock")
	l, err := Listen(addr, ListenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if _, err := Listen(addr, ListenOptions{}); !errors.Is(err, ErrInUse) {
		t.Fatalf("second Listen error = %v, want ErrInUse", err)
	}
}

func TestListenRefusesRegularFile(t *testing.T) {
	addr := filepath.Join(shortDir(t), "s.sock")
	if err := os.WriteFile(addr, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(addr, ListenOptions{}); err == nil {
		t.Fatal("Listen succeeded over a regular file")
	}
	if _, err := os.Stat(addr); err != nil {
		t.Fatalf("regular file was removed: %v", err)
	}
}
