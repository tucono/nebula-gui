// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

// fakePinger answers from a table. With block set, Ping reports on started
// and waits for block to close.
type fakePinger struct {
	mu      sync.Mutex
	results map[netip.Addr]PingResult
	err     error
	calls   []string // "addr count timeout"
	block   chan struct{}
	started chan netip.Addr
}

func (p *fakePinger) Ping(ctx context.Context, addr netip.Addr, count int, timeout time.Duration) (PingResult, error) {
	p.mu.Lock()
	p.calls = append(p.calls, addr.String()+" "+itoa(count)+" "+timeout.String())
	res, err, block, started := p.results[addr], p.err, p.block, p.started
	p.mu.Unlock()
	if block != nil {
		started <- addr
		select {
		case <-block:
		case <-ctx.Done():
			return PingResult{}, ctx.Err()
		}
	}
	if err != nil {
		return PingResult{}, err
	}
	if res.Sent == 0 {
		res.Sent = count
	}
	return res, nil
}

func itoa(n int) string { return strconv.Itoa(n) }

// pingServer returns a server with Nebula (fake) running and a fake pinger.
// The fake engine's host is 192.168.100.20/24.
func pingServer(t *testing.T) (*Server, http.Handler, *fakeEngine, *fakePinger) {
	t.Helper()
	e := &fakeEngine{}
	srv, _ := newTestServerWith(t, e, t.TempDir())
	p := &fakePinger{results: map[netip.Addr]PingResult{}}
	srv.pinger = p
	h := srv.Handler()
	putConfig(t, h, testConfig())
	if code, ec := call(t, h, http.MethodPost, "/v1/connect", nil, nil); code != http.StatusOK {
		t.Fatalf("connect = %d %s", code, ec)
	}
	return srv, h, e, p
}

// pingErr sends a ping and returns the status, error code and reason.
func pingErr(t *testing.T, h http.Handler, body string) (int, api.ErrorCode, api.Reason) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/ping", strings.NewReader(body)))
	if rec.Code < 400 {
		return rec.Code, "", ""
	}
	e := decodeErrBody(t, rec.Body.String())
	return rec.Code, e.Code, e.Reason
}

func decodeErrBody(t *testing.T, s string) api.Error {
	t.Helper()
	var eb api.ErrorBody
	if err := json.Unmarshal([]byte(s), &eb); err != nil || eb.Error == nil {
		t.Fatalf("bad error body %q", s)
	}
	return *eb.Error
}

func TestPingNotRunning(t *testing.T) {
	h := newTestServer(t).Handler()
	if code, ec, _ := pingErr(t, h, `{"vpn_ip":"192.168.100.10"}`); code != http.StatusConflict || ec != api.CodeNotRunning {
		t.Fatalf("ping while stopped = %d %s", code, ec)
	}
}

func TestPingValidation(t *testing.T) {
	_, h, _, p := pingServer(t)
	for _, tc := range []struct {
		body   string
		reason api.Reason
	}{
		{`not json`, ""},
		{`{"vpn_ip":"alice"}`, ""},
		{`{}`, ""},
		{`{"vpn_ip":"10.0.0.1"}`, api.ReasonNotInOverlay},
		{`{"vpn_ip":"192.168.101.10"}`, api.ReasonNotInOverlay},
		{`{"vpn_ip":"192.168.100.20"}`, api.ReasonOwnAddress},
		{`{"vpn_ip":"192.168.100.10","count":11}`, ""},
		{`{"vpn_ip":"192.168.100.10","count":-1}`, ""},
		{`{"vpn_ip":"192.168.100.10","timeout_ms":99}`, ""},
		{`{"vpn_ip":"192.168.100.10","timeout_ms":15001}`, ""},
	} {
		code, ec, reason := pingErr(t, h, tc.body)
		if code != http.StatusBadRequest || ec != api.CodeBadRequest || reason != tc.reason {
			t.Errorf("ping %s = %d %s %q, want 400 bad_request %q", tc.body, code, ec, reason, tc.reason)
		}
	}
	if len(p.calls) != 0 {
		t.Fatalf("pinger called for invalid requests: %q", p.calls)
	}
}

func TestPingDefaultsAndResult(t *testing.T) {
	_, h, _, p := pingServer(t)
	alice := netip.MustParseAddr("192.168.100.10")
	p.results[alice] = PingResult{Sent: 3, Received: 2,
		MinRTT: 21 * time.Millisecond, AvgRTT: 23440 * time.Microsecond, MaxRTT: 27960 * time.Microsecond}

	var r api.PingResponse
	if code, ec := call(t, h, http.MethodPost, "/v1/ping", api.PingRequest{VpnIP: "192.168.100.10"}, &r); code != http.StatusOK {
		t.Fatalf("ping = %d %s", code, ec)
	}
	if r.VpnIP != "192.168.100.10" || r.Sent != 3 || r.Received != 2 ||
		r.RTTMinMs == nil || *r.RTTMinMs != 21 || *r.RTTAvgMs != 23.4 || *r.RTTMaxMs != 28 {
		t.Fatalf("ping response = %+v", r)
	}
	call(t, h, http.MethodPost, "/v1/ping", api.PingRequest{VpnIP: "192.168.100.10", Count: 10, TimeoutMs: 100}, nil)
	want := []string{"192.168.100.10 3 5s", "192.168.100.10 10 100ms"}
	if len(p.calls) != 2 || p.calls[0] != want[0] || p.calls[1] != want[1] {
		t.Fatalf("pinger calls = %q, want %q", p.calls, want)
	}
}

func TestPingNoReplies(t *testing.T) {
	_, h, e, _ := pingServer(t)
	e.instance().setHosts([]nebulactl.HostInfo{host("192.168.100.11", "bob")}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/ping", strings.NewReader(`{"vpn_ip":"192.168.100.11"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping = %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "rtt_") || !strings.Contains(body, `"received":0`) {
		t.Fatalf("no-reply body = %s", body)
	}
	if p := peers(t, h); len(p) != 1 || p[0].LastPingRTTMs != nil {
		t.Fatalf("peers after an unanswered ping = %+v", p)
	}
}

func TestPingFailed(t *testing.T) {
	_, h, _, p := pingServer(t)
	p.err = errors.New("socket: operation not permitted")
	code, ec, _ := pingErr(t, h, `{"vpn_ip":"192.168.100.10"}`)
	if code != http.StatusInternalServerError || ec != api.CodePingFailed {
		t.Fatalf("ping with a failing pinger = %d %s", code, ec)
	}
}

func TestPingRTTInPeers(t *testing.T) {
	_, h, e, p := pingServer(t)
	alice := netip.MustParseAddr("192.168.100.10")
	p.results[alice] = PingResult{Received: 3, AvgRTT: 23400 * time.Microsecond}
	e.instance().setHosts([]nebulactl.HostInfo{host("192.168.100.10", "alice"), host("192.168.100.11", "bob")}, nil)

	call(t, h, http.MethodPost, "/v1/ping", api.PingRequest{VpnIP: "192.168.100.10"}, nil)
	ps := peers(t, h)
	if len(ps) != 2 || ps[0].LastPingRTTMs == nil || *ps[0].LastPingRTTMs != 23.4 || ps[1].LastPingRTTMs != nil {
		t.Fatalf("peers after ping = %+v", ps)
	}

	// Disconnecting clears the RTTs.
	call(t, h, http.MethodPost, "/v1/disconnect", nil, nil)
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	e.instance().setHosts([]nebulactl.HostInfo{host("192.168.100.10", "alice")}, nil)
	if ps := peers(t, h); len(ps) != 1 || ps[0].LastPingRTTMs != nil {
		t.Fatalf("peers after reconnect = %+v", ps)
	}
}

func TestPingRTTNotKeptAcrossRestart(t *testing.T) {
	_, h, e, p := pingServer(t)
	alice := netip.MustParseAddr("192.168.100.10")
	p.results[alice] = PingResult{Received: 1, AvgRTT: 10 * time.Millisecond}
	p.block, p.started = make(chan struct{}), make(chan netip.Addr, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		call(t, h, http.MethodPost, "/v1/ping", api.PingRequest{VpnIP: "192.168.100.10"}, nil)
	}()
	<-p.started
	// Nebula restarts while the ping runs; its result must not show up
	// against the new instance.
	call(t, h, http.MethodPost, "/v1/disconnect", nil, nil)
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	close(p.block)
	<-done
	e.instance().setHosts([]nebulactl.HostInfo{host("192.168.100.10", "alice")}, nil)
	if ps := peers(t, h); ps[0].LastPingRTTMs != nil {
		t.Fatalf("RTT from before the restart kept: %+v", ps[0])
	}
}

func TestPingRateLimitedHTTP(t *testing.T) {
	_, h, _, p := pingServer(t)
	p.block, p.started = make(chan struct{}), make(chan netip.Addr, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		call(t, h, http.MethodPost, "/v1/ping", api.PingRequest{VpnIP: "192.168.100.10"}, nil)
	}()
	<-p.started
	if code, ec, _ := pingErr(t, h, `{"vpn_ip":"192.168.100.10"}`); code != http.StatusTooManyRequests || ec != api.CodeRateLimited {
		t.Fatalf("second ping to the same peer = %d %s", code, ec)
	}
	close(p.block)
	<-done
}

func TestPingLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newPingLimiter()
	l.now = func() time.Time { return now }
	ip := func(n int) netip.Addr { return netip.MustParseAddr("192.168.100." + strconv.Itoa(n)) }

	// One per peer.
	rel, ok := l.acquire(ip(1))
	if !ok {
		t.Fatal("first ping refused")
	}
	if _, ok := l.acquire(ip(1)); ok {
		t.Fatal("second ping to the same peer allowed")
	}
	rel()
	if _, ok := l.acquire(ip(1)); !ok {
		t.Fatal("ping after release refused")
	}

	// At most four in flight. ip(1) is still in flight.
	var rels []func()
	for n := 2; n <= 4; n++ {
		r, ok := l.acquire(ip(n))
		if !ok {
			t.Fatalf("ping %d refused", n)
		}
		rels = append(rels, r)
	}
	if _, ok := l.acquire(ip(5)); ok {
		t.Fatal("fifth ping in flight allowed")
	}
	for _, r := range rels {
		r()
	}

	// At most 60 starts per minute, on a fresh limiter.
	l2 := newPingLimiter()
	l2.now = func() time.Time { return now }
	for n := range 60 {
		r, ok := l2.acquire(ip(10 + n))
		if !ok {
			t.Fatalf("ping %d of 60 refused", n+1)
		}
		r()
	}
	if _, ok := l2.acquire(ip(100)); ok {
		t.Fatal("61st ping in a minute allowed")
	}
	now = now.Add(time.Minute)
	if _, ok := l2.acquire(ip(100)); !ok {
		t.Fatal("ping a minute later refused")
	}
}

func TestCheckPingTarget(t *testing.T) {
	nets := []netip.Prefix{netip.MustParsePrefix("192.168.100.20/24")}
	for _, tc := range []struct {
		ip     string
		reason api.Reason
	}{
		{"192.168.100.1", ""},
		{"192.168.100.255", ""},
		{"192.168.100.20", api.ReasonOwnAddress},
		{"192.168.99.1", api.ReasonNotInOverlay},
		{"fd00::1", api.ReasonNotInOverlay},
	} {
		err := checkPingTarget(netip.MustParseAddr(tc.ip), nets)
		var got api.Reason
		var br *errBadRequest
		if errors.As(err, &br) {
			got = br.reason
		} else if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.ip, err)
		}
		if got != tc.reason {
			t.Errorf("%s: reason %q, want %q", tc.ip, got, tc.reason)
		}
	}
}
