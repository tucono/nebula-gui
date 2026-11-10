// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/ipc"
)

// TestServeOverSocket runs the server on a real Unix socket and talks to it
// with the API client, then checks that shutdown removes the socket.
func TestServeOverSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "svc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	addr := filepath.Join(dir, "s.sock")

	l, err := ipc.Listen(addr, ipc.ListenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- newTestServer().Serve(ctx, l) }()

	c := api.NewClient(addr)
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	v, err := c.Version(rctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.ServiceVersion != "1.2.3-test" {
		t.Fatalf("unexpected version: %+v", v)
	}
	if _, err := c.Status(rctx); err != nil {
		t.Fatalf("Status: %v", err)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if _, err := os.Lstat(addr); !os.IsNotExist(err) {
		t.Fatalf("socket still present after shutdown: %v", err)
	}
}
