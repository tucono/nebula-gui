// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"net"
	"os"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// fakeService answers like the service, with values the test sets.
type fakeService struct {
	mu        sync.Mutex
	down      bool
	apiVer    int
	instance  string
	state     api.State
	configErr error   // returned by Connect and Disconnect when set
	putErrs   []error // returned by the next PutConfig calls, in order
	put       []string
	unconfig  bool
	calls     []string
}

func newFakeService() *fakeService {
	return &fakeService{apiVer: api.APIVersion, instance: "i1", state: api.StateStopped}
}

var errRefused = &net.OpError{Op: "dial", Net: "unix", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}

func (f *fakeService) Version(context.Context) (*api.VersionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "version")
	if f.down {
		return nil, errRefused
	}
	return &api.VersionResponse{ServiceVersion: "0.2.0", NebulaVersion: "1.12.0", APIVersion: f.apiVer, InstanceID: f.instance}, nil
}

func (f *fakeService) Status(context.Context) (*api.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "status")
	if f.down {
		return nil, errRefused
	}
	return f.statusLocked(), nil
}

func (f *fakeService) statusLocked() *api.StatusResponse {
	if f.unconfig {
		return &api.StatusResponse{State: f.state, InstanceID: f.instance}
	}
	return &api.StatusResponse{State: f.state, Configured: true, InstanceID: f.instance, OverlayIPs: []string{"192.168.100.20/24"}}
}

func (f *fakeService) PutConfig(_ context.Context, yaml string) (*api.ConfigPutResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "put")
	if len(f.putErrs) > 0 {
		err := f.putErrs[0]
		f.putErrs = f.putErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	f.put = append(f.put, yaml)
	f.unconfig = false
	return &api.ConfigPutResponse{RestartRequired: f.state == api.StateRunning}, nil
}

func (f *fakeService) DeleteConfig(context.Context) (*api.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "delete")
	f.unconfig, f.state = true, api.StateStopped
	return f.statusLocked(), nil
}

func (f *fakeService) Connect(context.Context) (*api.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "connect")
	if f.configErr != nil {
		return nil, f.configErr
	}
	f.state = api.StateRunning
	return f.statusLocked(), nil
}

func (f *fakeService) Disconnect(context.Context) (*api.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "disconnect")
	if f.state != api.StateRunning && f.state != api.StateError {
		return nil, &api.Error{Code: api.CodeNotRunning, Message: "nebula is not running"}
	}
	f.state = api.StateStopped
	return f.statusLocked(), nil
}

func (f *fakeService) Addr() string { return "/run/user/1000/nebula-guid.sock" }

func (f *fakeService) set(fn func(f *fakeService)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeService) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

func msgs(l *appLog) []string {
	var out []string
	for _, e := range l.Recent(appLogKeep) {
		out = append(out, e.Msg)
	}
	return out
}

func equal(a, b []string) bool { return slices.Equal(a, b) }

func TestBackoff(t *testing.T) {
	want := []time.Duration{2, 4, 8, 10, 10}
	for i, w := range want {
		if got := backoff(i + 1); got != w*time.Second {
			t.Errorf("backoff(%d) = %v, want %v", i+1, got, w*time.Second)
		}
	}
}

func TestPollerSequence(t *testing.T) {
	svc := newFakeService()
	log := newAppLog()
	p := newPoller(svc, log, &statusGate{})
	ctx := context.Background()

	// First poll: version, then status.
	r, d := p.poll(ctx)
	if r.Version == nil || r.Status == nil || r.Err != nil || d != pollInterval {
		t.Fatalf("first poll: %+v, %v", r, d)
	}
	if c := svc.takeCalls(); !equal(c, []string{"version", "status"}) {
		t.Fatalf("first poll calls %v", c)
	}

	// Then status only.
	r, _ = p.poll(ctx)
	if r.Version != nil || r.Status == nil {
		t.Fatalf("second poll: %+v", r)
	}
	if c := svc.takeCalls(); !equal(c, []string{"status"}) {
		t.Fatalf("second poll calls %v", c)
	}

	// The service goes away: back off, and log it once.
	svc.set(func(f *fakeService) { f.down = true })
	var delays []time.Duration
	for range 5 {
		r, d = p.poll(ctx)
		if r.Err == nil {
			t.Fatal("no error while down")
		}
		delays = append(delays, d)
	}
	if delays[0] != 2*time.Second || delays[3] != 10*time.Second || delays[4] != 10*time.Second {
		t.Fatalf("delays %v", delays)
	}
	svc.takeCalls()

	// It comes back: version is checked again, and the backoff resets.
	svc.set(func(f *fakeService) { f.down = false; f.state = api.StateRunning })
	r, d = p.poll(ctx)
	if r.Version == nil || r.Status == nil || d != pollInterval {
		t.Fatalf("after recovery: %+v, %v", r, d)
	}
	if c := svc.takeCalls(); !equal(c, []string{"version", "status"}) {
		t.Fatalf("after recovery calls %v", c)
	}

	want := []string{"service reachable", "state changed", "service unreachable", "service reachable", "state changed"}
	if got := msgs(log); !equal(got, want) {
		t.Fatalf("log %v, want %v", got, want)
	}
}

func TestPollerRestart(t *testing.T) {
	svc := newFakeService()
	log := newAppLog()
	p := newPoller(svc, log, &statusGate{})
	ctx := context.Background()
	p.poll(ctx)
	svc.takeCalls()

	// A new instance ID means the service restarted: check the version.
	svc.set(func(f *fakeService) { f.instance = "i2" })
	r, _ := p.poll(ctx)
	if r.Version == nil || r.Version.InstanceID != "i2" || r.Status == nil {
		t.Fatalf("after restart: %+v", r)
	}
	if c := svc.takeCalls(); !equal(c, []string{"status", "version"}) {
		t.Fatalf("calls %v", c)
	}
	r, _ = p.poll(ctx)
	if r.Version != nil {
		t.Fatal("checked the version again without a restart")
	}
	if got := msgs(log); !equal(got, []string{"service reachable", "state changed", "service restarted"}) {
		t.Fatalf("log %v", got)
	}

	// The restarted service speaks another API version: no status, and
	// polling slows down until it changes.
	svc.set(func(f *fakeService) { f.instance = "i3"; f.apiVer = api.APIVersion + 1 })
	r, d := p.poll(ctx)
	if r.Version == nil || r.Status != nil || r.Err != nil || d != maxPollDelay {
		t.Fatalf("incompatible: %+v, %v", r, d)
	}
	svc.takeCalls()
	r, _ = p.poll(ctx)
	if r.Version == nil || r.Status != nil {
		t.Fatalf("incompatible again: %+v", r)
	}
	if c := svc.takeCalls(); !equal(c, []string{"version"}) {
		t.Fatalf("incompatible calls %v", c)
	}
	n := 0
	for _, m := range msgs(log) {
		if m == "incompatible service" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("logged the mismatch %d times", n)
	}
}

func TestPollerRunAndPoke(t *testing.T) {
	svc := newFakeService()
	p := newPoller(svc, newAppLog(), &statusGate{})
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan pollResult, 10)
	done := make(chan struct{})
	go func() {
		p.run(ctx, func(r pollResult) { results <- r })
		close(done)
	}()
	<-results
	// The next poll is 2 s away; Poke makes it happen now.
	p.Poke()
	select {
	case <-results:
	case <-time.After(time.Second):
		t.Fatal("Poke didn't poll")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run didn't stop")
	}
}

func TestStatusGate(t *testing.T) {
	g := &statusGate{}
	poll := g.Begin()   // a poll is sent…
	action := g.Begin() // …then Connect's response arrives
	if !g.Accept(action) {
		t.Fatal("rejected the action's status")
	}
	if g.Accept(poll) {
		t.Fatal("accepted the older poll after the action")
	}
	if !g.Accept(g.Begin()) {
		t.Fatal("rejected a newer poll")
	}
}
