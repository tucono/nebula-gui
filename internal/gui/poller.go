// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	// pollInterval is how often the status is read while the service
	// answers.
	pollInterval = 2 * time.Second
	// maxPollDelay caps the backoff while the service can't be reached,
	// and is the interval while it speaks another API version.
	maxPollDelay = 10 * time.Second
	// readTimeout is for calls that only read: Version, Status, Logs,
	// Diagnostics and Config.
	readTimeout = 5 * time.Second
	// actionTimeout is for calls that change something. Disconnect can
	// take more than 10 s while Nebula's packet readers exit, and a slow
	// but successful action must not show "isn't responding".
	actionTimeout = 30 * time.Second
	// peersSlowInterval is how often peers are read while the Peers tab
	// isn't showing. While it shows, they're read on every poll.
	peersSlowInterval = 10 * time.Second
)

// serviceClient is the part of api.Client the poller uses.
type serviceClient interface {
	Version(ctx context.Context) (*api.VersionResponse, error)
	Status(ctx context.Context) (*api.StatusResponse, error)
	Peers(ctx context.Context) (*api.PeersResponse, error)
}

// pollResult is what one poll learned. Exactly one of Err and Status is
// set, except while the service speaks another API version: then only
// Version is set.
type pollResult struct {
	// Seq orders this result against Connect and Disconnect responses;
	// see statusGate.
	Seq uint64
	// Version is set when the poll asked for it: after the service was
	// unreachable, and when it restarted.
	Version *api.VersionResponse
	Status  *api.StatusResponse
	Err     error
	// PeersUpdated says Peers is a fresh list: read this round while
	// Nebula runs, or empty because it doesn't. When it's false, the
	// previous list still stands.
	PeersUpdated bool
	Peers        []api.Peer
}

type reachability int

const (
	reachUnknown reachability = iota
	reachUp
	reachDown
)

// poller asks the service for its status: Version once after each
// (re)connect, then Status every pollInterval, backing off 2 → 4 → 8 → 10 s
// while the service can't be reached. It notices a service restart by the
// instance ID and checks the version again. It records changes, not every
// poll, in the GUI log.
//
// While Nebula runs it also reads the peers: on every poll while peersFast
// is set (the Peers tab shows), otherwise every peersSlowInterval, and at
// once after Poke.
type poller struct {
	c    serviceClient
	log  *appLog
	gate *statusGate
	wake chan struct{}
	now  func() time.Time

	peersFast atomic.Bool
	peersNow  atomic.Bool // Poke asked for fresh peers

	// Only the polling goroutine touches these.
	versionOK  bool // the version was checked since the last (re)connect
	mismatch   int  // API version last logged as incompatible, or 0
	instance   string
	reach      reachability
	lastState  api.State
	lastConfig bool
	failures   int
	peersAt    time.Time // when peers were last read, zero if not yet
	peersErr   bool      // the last peers read failed, and that was logged
}

func newPoller(c serviceClient, log *appLog, gate *statusGate) *poller {
	return &poller{c: c, log: log, gate: gate, wake: make(chan struct{}, 1), now: time.Now}
}

// SetPeersFast switches between reading peers on every poll and every
// peersSlowInterval. Switching to fast polls at once.
func (p *poller) SetPeersFast(on bool) {
	if p.peersFast.Swap(on) != on && on {
		p.Poke()
	}
}

// Poke makes the poller poll now instead of waiting, and read the peers
// too.
func (p *poller) Poke() {
	p.peersNow.Store(true)
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// run polls until ctx ends, passing each result to deliver on the polling
// goroutine.
func (p *poller) run(ctx context.Context, deliver func(pollResult)) {
	for {
		r, delay := p.poll(ctx)
		if ctx.Err() != nil {
			return
		}
		deliver(r)
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-p.wake:
			t.Stop()
		}
	}
}

// poll does one round and returns how long to wait before the next.
func (p *poller) poll(ctx context.Context) (pollResult, time.Duration) {
	r := pollResult{Seq: p.gate.Begin()}
	if !p.versionOK {
		v, err := p.version(ctx)
		if err != nil {
			return p.failed(r, err)
		}
		r.Version = v
		if v.APIVersion != api.APIVersion {
			return r, maxPollDelay
		}
	}

	st, err := p.status(ctx)
	if err != nil {
		return p.failed(r, err)
	}
	if p.instance != "" && st.InstanceID != "" && st.InstanceID != p.instance {
		// The service restarted, for example after an upgrade. Ask for
		// the version again before trusting the status.
		p.log.Add("service restarted")
		p.instance = ""
		v, err := p.version(ctx)
		if err != nil {
			return p.failed(r, err)
		}
		r.Version = v
		if v.APIVersion != api.APIVersion {
			return r, maxPollDelay
		}
	}
	if st.InstanceID != "" {
		p.instance = st.InstanceID
	}
	p.noteStatus(st)
	r.Status = st
	p.failures = 0
	p.pollPeers(ctx, st, &r)
	return r, pollInterval
}

// pollPeers reads the peers into r when they're due. A failed read keeps
// the previous list: the status answered, so the service is up, and the
// next poll tries again.
func (p *poller) pollPeers(ctx context.Context, st *api.StatusResponse, r *pollResult) {
	if st.State != api.StateRunning {
		p.peersAt = time.Time{}
		p.peersNow.Store(false)
		r.PeersUpdated, r.Peers = true, nil
		return
	}
	now := p.now()
	forced := p.peersNow.Swap(false)
	if !forced && !p.peersFast.Load() && !p.peersAt.IsZero() && now.Sub(p.peersAt) < peersSlowInterval {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	resp, err := p.c.Peers(ctx)
	if err != nil {
		if !p.peersErr {
			p.peersErr = true
			p.log.Add("couldn't read peers", "error", err.Error())
		}
		return
	}
	p.peersErr = false
	p.peersAt = now
	r.PeersUpdated, r.Peers = true, resp.Peers
}

func (p *poller) version(ctx context.Context) (*api.VersionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	v, err := p.c.Version(ctx)
	if err != nil {
		return nil, err
	}
	if p.reach != reachUp {
		p.reach = reachUp
		p.log.Add("service reachable", "service", v.ServiceVersion, "nebula", v.NebulaVersion,
			"api", fmt.Sprint(v.APIVersion))
	}
	if v.APIVersion != api.APIVersion {
		if p.mismatch != v.APIVersion {
			p.mismatch = v.APIVersion
			p.log.Add("incompatible service", "service_api", fmt.Sprint(v.APIVersion), "app_api", fmt.Sprint(api.APIVersion))
		}
		p.versionOK = false
		return v, nil
	}
	p.mismatch = 0
	p.versionOK = true
	p.instance = v.InstanceID
	return v, nil
}

func (p *poller) status(ctx context.Context) (*api.StatusResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	return p.c.Status(ctx)
}

// failed records an unreachable service and backs off.
func (p *poller) failed(r pollResult, err error) (pollResult, time.Duration) {
	if p.reach != reachDown {
		p.reach = reachDown
		p.log.Add("service unreachable", "problem", string(problemFor(err, problemContext{}).ID), "error", err.Error())
	}
	// Check the version again once it answers: it may be a new service.
	p.versionOK = false
	p.instance = ""
	p.lastState = ""
	p.peersAt = time.Time{}
	p.failures++
	r.Err = err
	return r, backoff(p.failures)
}

// noteStatus logs changes of state and of whether a config is stored.
func (p *poller) noteStatus(st *api.StatusResponse) {
	if st.Configured != p.lastConfig || p.lastState == "" {
		if !st.Configured {
			p.log.Add("no config stored")
		}
		p.lastConfig = st.Configured
	}
	if st.State == p.lastState {
		return
	}
	kv := []string{"state", string(st.State)}
	if p.lastState != "" {
		kv = append(kv, "was", string(p.lastState))
	}
	if st.State == api.StateError {
		kv = append(kv, "reason", string(st.ErrorReason), "error", st.Error)
	}
	p.log.Add("state changed", kv...)
	p.lastState = st.State
}

// backoff is the wait after the nth failure in a row: 2, 4, 8, then 10 s.
func backoff(failures int) time.Duration {
	d := pollInterval
	for i := 1; i < failures && d < maxPollDelay; i++ {
		d *= 2
	}
	return min(d, maxPollDelay)
}

// statusGate keeps a slow, stale status from overwriting a newer one. A
// poll takes a sequence number when it sends its request; an action takes
// one when its response arrives, because the response shows the state
// after the action. The window applies a status only if its number is
// newer than the last one applied. Without this, a poll sent just before
// Connect finished could switch the header back to "Connecting…".
type statusGate struct {
	mu      sync.Mutex
	next    uint64
	applied uint64
}

// Begin returns the next sequence number.
func (g *statusGate) Begin() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next
}

// Accept reports whether a result numbered seq is newer than the last one
// accepted, and if so records it.
func (g *statusGate) Accept(seq uint64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if seq <= g.applied {
		return false
	}
	g.applied = seq
	return true
}
