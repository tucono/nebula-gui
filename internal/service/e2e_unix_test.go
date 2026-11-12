// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix

package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/ipc"
	"github.com/tucono/nebula-gui/internal/nebulactl/nebulatest"
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
	go func() { done <- newTestServer(t).Serve(ctx, l) }()

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

// TestRealNebulaOverSocket imports a generated config, connects and
// disconnects a real embedded Nebula (tun disabled, so no privileges),
// all through the API client.
func TestRealNebulaOverSocket(t *testing.T) {
	nebulatest.SkipIfRace(t)
	p, err := nebulatest.NewPKI("dev", "192.168.100.20/24", "users")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "svc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	addr := filepath.Join(dir, "s.sock")

	store, err := OpenStore(filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	logs := NewLogBuffer(100)
	logger := slog.New(logs.Handler(slog.LevelInfo))
	srv, err := New(Options{
		Version: "test",
		Logger:  logger,
		Engine:  NebulaEngine{BuildVersion: "test", Logger: logger},
		Store:   store,
		Logs:    logs,
	})
	if err != nil {
		t.Fatal(err)
	}
	l, err := ipc.Listen(addr, ipc.ListenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})

	c := api.NewClient(addr)
	rctx, rcancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer rcancel()

	v, err := c.Version(rctx)
	if err != nil || v.NebulaVersion == "" {
		t.Fatalf("Version = %+v, %v", v, err)
	}
	if _, err := c.PutConfig(rctx, p.Config()); err != nil {
		t.Fatalf("PutConfig: %v", err)
	}
	s, err := c.Connect(rctx)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if s.State != api.StateRunning || s.Cert == nil || s.Cert.Name != "dev" ||
		len(s.OverlayIPs) != 1 || s.OverlayIPs[0] != "192.168.100.20/24" {
		t.Fatalf("status = %+v", s)
	}

	lr, err := c.Logs(rctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range lr.Lines {
		if line.Msg == "Nebula interface is active" && line.Fields["component"] == "nebula" {
			found = true
		}
		if strings.Contains(line.Msg, "PRIVATE") {
			t.Fatalf("private key in the log: %q", line.Msg)
		}
	}
	if !found {
		t.Errorf("nebula startup line missing from logs: %+v", lr.Lines)
	}

	time.Sleep(200 * time.Millisecond) // see nebulactl.TestStartStop
	if s, err = c.Disconnect(rctx); err != nil || s.State != api.StateStopped {
		t.Fatalf("Disconnect = %+v, %v", s, err)
	}
}
