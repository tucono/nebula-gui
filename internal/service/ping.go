// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/tucono/nebula-gui/internal/api"
)

// Ping rate limits. See docs/ipc-api.md.
const (
	pingMaxInFlight = 4
	pingMaxPerMin   = 60
)

var errRateLimited = errors.New("too many pings, try again in a moment")

// errBadRequest is a bad_request with a reason.
type errBadRequest struct {
	reason api.Reason
	msg    string
}

func (e *errBadRequest) Error() string { return e.msg }

// errPingFailed means the echo requests couldn't be sent at all.
type errPingFailed struct{ err error }

func (e *errPingFailed) Error() string { return "ping failed: " + e.err.Error() }
func (e *errPingFailed) Unwrap() error { return e.err }

// PingResult is the outcome of one ping run.
type PingResult struct {
	Sent, Received         int
	MinRTT, AvgRTT, MaxRTT time.Duration
}

// pinger sends ICMP echo requests. The real one uses pro-bing; tests use a
// fake.
type pinger interface {
	Ping(ctx context.Context, addr netip.Addr, count int, timeout time.Duration) (PingResult, error)
}

// proBingPinger pings with raw ICMP sockets, so the service needs root,
// SYSTEM or cap_net_raw.
type proBingPinger struct{}

func (proBingPinger) Ping(ctx context.Context, addr netip.Addr, count int, timeout time.Duration) (PingResult, error) {
	p, err := probing.NewPinger(addr.String())
	if err != nil {
		return PingResult{}, err
	}
	p.SetPrivileged(true)
	p.Count = count
	p.Timeout = timeout
	// Spread the requests so they all go out before the timeout.
	p.Interval = max(min(time.Second, timeout/time.Duration(count+1)), 50*time.Millisecond)
	if err := p.RunWithContext(ctx); err != nil {
		return PingResult{}, err
	}
	st := p.Statistics()
	return PingResult{
		Sent: st.PacketsSent, Received: st.PacketsRecv,
		MinRTT: st.MinRtt, AvgRTT: st.AvgRtt, MaxRTT: st.MaxRtt,
	}, nil
}

// pingLimiter enforces the ping rate limits.
type pingLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	inFlight map[netip.Addr]bool
	starts   []time.Time // accepted starts in the last minute, oldest first
}

func newPingLimiter() *pingLimiter {
	return &pingLimiter{now: time.Now, inFlight: map[netip.Addr]bool{}}
}

// acquire reserves a ping to addr. It returns false when a limit is hit;
// otherwise the caller must call release when the ping is done.
func (l *pingLimiter) acquire(addr netip.Addr) (release func(), ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cut := 0
	for cut < len(l.starts) && now.Sub(l.starts[cut]) >= time.Minute {
		cut++
	}
	l.starts = l.starts[cut:]
	if len(l.inFlight) >= pingMaxInFlight || l.inFlight[addr] || len(l.starts) >= pingMaxPerMin {
		return nil, false
	}
	l.inFlight[addr] = true
	l.starts = append(l.starts, now)
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.inFlight, addr)
	}, true
}

// checkPingTarget checks that addr is inside one of the networks and isn't
// one of this host's own addresses. networks are the host's cert networks,
// such as 192.168.100.20/24.
func checkPingTarget(addr netip.Addr, networks []netip.Prefix) error {
	inside := false
	for _, n := range networks {
		if n.Addr() == addr {
			return &errBadRequest{reason: api.ReasonOwnAddress, msg: "vpn_ip is this host's own address"}
		}
		if n.Masked().Contains(addr) {
			inside = true
		}
	}
	if !inside {
		return &errBadRequest{reason: api.ReasonNotInOverlay, msg: "vpn_ip is not inside this host's Nebula network"}
	}
	return nil
}

// pingParams applies the defaults and checks the ranges.
func pingParams(req api.PingRequest) (count int, timeout time.Duration, err error) {
	count, ms := req.Count, req.TimeoutMs
	if count == 0 {
		count = api.PingDefaultCount
	}
	if ms == 0 {
		ms = api.PingDefaultTimeoutMs
	}
	if count < 1 || count > api.PingMaxCount {
		return 0, 0, fmt.Errorf("count must be 1-%d", api.PingMaxCount)
	}
	if ms < api.PingMinTimeoutMs || ms > api.PingMaxTimeoutMs {
		return 0, 0, fmt.Errorf("timeout_ms must be %d-%d", api.PingMinTimeoutMs, api.PingMaxTimeoutMs)
	}
	return count, time.Duration(ms) * time.Millisecond, nil
}

// pingTarget returns the running instance and its cert networks.
func (m *manager) pingTarget() (Instance, []netip.Prefix, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != api.StateRunning || m.inst == nil || m.running == nil {
		return nil, nil, errNotRunning
	}
	nets := make([]netip.Prefix, 0, len(m.running.OverlayIPs))
	for _, s := range m.running.OverlayIPs {
		if p, err := netip.ParsePrefix(s); err == nil {
			nets = append(nets, p)
		}
	}
	return m.inst, nets, nil
}

// recordRTT keeps avgMs as the last RTT to addr, unless Nebula was stopped
// or restarted since inst was current.
func (m *manager) recordRTT(inst Instance, addr netip.Addr, avgMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inst != inst {
		return
	}
	if m.rtt == nil {
		m.rtt = map[netip.Addr]float64{}
	}
	m.rtt[addr] = avgMs
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	var req api.PingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		s.writeError(w, api.CodeBadRequest, "request body is not valid JSON")
		return
	}
	inst, nets, err := s.mgr.pingTarget()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	addr, err := netip.ParseAddr(req.VpnIP)
	if err != nil {
		s.writeError(w, api.CodeBadRequest, "vpn_ip is not an IP address")
		return
	}
	addr = addr.Unmap()
	if err := checkPingTarget(addr, nets); err != nil {
		s.writeErr(w, err)
		return
	}
	count, timeout, err := pingParams(req)
	if err != nil {
		s.writeError(w, api.CodeBadRequest, err.Error())
		return
	}
	release, ok := s.pingLimit.acquire(addr)
	if !ok {
		s.writeErr(w, errRateLimited)
		return
	}
	defer release()

	res, err := s.pinger.Ping(r.Context(), addr, count, timeout)
	if err != nil {
		s.log.Warn("ping failed", "vpn_ip", addr.String(), "err", err)
		s.writeErr(w, &errPingFailed{err: err})
		return
	}
	resp := api.PingResponse{VpnIP: addr.String(), Sent: res.Sent, Received: res.Received}
	if res.Received > 0 {
		lo, avg, hi := toMs(res.MinRTT), toMs(res.AvgRTT), toMs(res.MaxRTT)
		resp.RTTMinMs, resp.RTTAvgMs, resp.RTTMaxMs = &lo, &avg, &hi
		s.mgr.recordRTT(inst, addr, avg)
		s.log.Info("ping", "vpn_ip", resp.VpnIP, "sent", res.Sent, "received", res.Received, "rtt_avg_ms", avg)
	} else {
		s.log.Info("ping", "vpn_ip", resp.VpnIP, "sent", res.Sent, "received", 0)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// toMs converts d to milliseconds, rounded to 0.1 ms.
func toMs(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*10) / 10
}
