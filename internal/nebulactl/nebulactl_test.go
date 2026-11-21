// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl_test

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/nebulactl"
	"github.com/tucono/nebula-gui/internal/nebulactl/nebulatest"
)

func newPKI(t *testing.T) nebulatest.PKI {
	t.Helper()
	p, err := nebulatest.NewPKI("dev", "192.168.100.20/24", "users")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVersion(t *testing.T) {
	if v := nebulactl.Version(); v == "" || strings.HasPrefix(v, "v") {
		t.Fatalf("Version() = %q", v)
	}
}

func TestCheck(t *testing.T) {
	info, err := nebulactl.Check(newPKI(t).Config())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	c := info.Cert
	if c.Name != "dev" || len(c.Groups) != 1 || c.Groups[0] != "users" {
		t.Errorf("cert = %+v", c)
	}
	if len(c.Networks) != 1 || c.Networks[0].String() != "192.168.100.20/24" {
		t.Errorf("networks = %v", c.Networks)
	}
	if c.Fingerprint == "" || c.NotAfter.Before(time.Now()) {
		t.Errorf("cert = %+v", c)
	}
}

func TestCheckRejectsMismatchedKey(t *testing.T) {
	a, b := newPKI(t), newPKI(t)
	a.Key = b.Key
	if _, err := nebulactl.Check(a.Config()); err == nil {
		t.Fatal("Check accepted a key that doesn't match the cert")
	}
}

// TestStartStop runs a real Nebula with the tun device disabled, which
// needs no privileges.
func TestStartStop(t *testing.T) {
	nebulatest.SkipIfRace(t)
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	inst, err := nebulactl.Start(newPKI(t).Config(), "test", l)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-inst.Done():
		t.Fatalf("nebula stopped on its own: %v", inst.Err())
	default:
	}
	if hosts := inst.Hosts(false); len(hosts) != 0 {
		t.Errorf("hosts = %+v", hosts)
	}
	// Let the packet readers start: with tun.disabled, Nebula 1.12.0 leaves
	// a reader that starts after Close blocked forever.
	time.Sleep(200 * time.Millisecond)

	stopped := make(chan struct{})
	go func() { inst.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("Stop did not return")
	}
	select {
	case <-inst.Done():
	default:
		t.Fatal("Done not closed after Stop")
	}
	if err := inst.Err(); err != nil {
		t.Errorf("Err after Stop = %v", err)
	}
	inst.Stop() // a second Stop is a no-op
}
