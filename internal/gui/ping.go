// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"net/netip"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	// pingCallTimeout bounds one POST /v1/ping: the ping's own timeout
	// plus room for a slow service.
	pingCallTimeout = time.Duration(api.PingDefaultTimeoutMs)*time.Millisecond + 10*time.Second
	// problemWidth keeps a problem dialog from growing with its text.
	problemWidth = 400
)

// Texts of ping results. The Peers tab puts the peer's name in front.
const (
	pingTextConnecting = "Connecting…"
	pingTextFirewall   = "Connected, but it didn't answer. Its firewall may not allow ping."
	pingTextNoAnswer   = "No answer. It may be switched off or not connected to Nebula, or the address may be wrong."
	pingTextBusy       = "Too many pings at once. Try again in a moment."
	pingTextPinging    = "Pinging…"
)

// pingKind is how a ping result is shown.
type pingKind int

const (
	pingAnswered pingKind = iota // some replies
	pingWaiting                  // connecting, or busy: try again
	pingNoAnswer                 // no replies
	pingError                    // the ping didn't run; Problem says why
)

// pingView is what the Peers tab shows after a ping.
type pingView struct {
	Kind pingKind
	Text string
	// Retry says to ping once more: the first ping opened a tunnel, and
	// its replies were probably lost while Nebula handshook.
	Retry bool
	// Problem is set for pingError.
	Problem *problem
}

// describePing judges a ping by its replies and by the peer's tunnel
// before and after it (nil when there was none). A retry passes the
// tunnel after the first ping as before, so it never asks for a second
// retry. A tunnel still pending after a ping without replies counts as
// no answer: Nebula keeps a pending tunnel for an address nobody has
// while it asks the lighthouse, and a real peer finishes its handshake
// well within a ping's timeout.
func describePing(before, after *api.Peer, res *api.PingResponse, err error) pingView {
	if err != nil {
		if api.ErrorCodeOf(err) == api.CodeRateLimited {
			return pingView{Kind: pingWaiting, Text: pingTextBusy}
		}
		p := problemFor(err, problemContext{Op: opPing})
		return pingView{Kind: pingError, Text: p.Title, Problem: p}
	}
	if res != nil && res.Received > 0 {
		s := "Answered"
		if res.RTTAvgMs != nil {
			s += " in " + formatMs(*res.RTTAvgMs)
		}
		if res.Received < res.Sent {
			s += ", " + strconv.Itoa(res.Received) + " of " + strconv.Itoa(res.Sent) + " replies"
		}
		return pingView{Kind: pingAnswered, Text: s + "."}
	}
	switch {
	case before == nil && after != nil:
		return pingView{Kind: pingWaiting, Text: pingTextConnecting, Retry: true}
	case after != nil && after.State == api.PeerEstablished:
		return pingView{Kind: pingNoAnswer, Text: pingTextFirewall}
	default:
		return pingView{Kind: pingNoAnswer, Text: pingTextNoAnswer}
	}
}

// findPeer returns the peer with ip among its overlay IPs, or nil.
func findPeer(peers []api.Peer, ip netip.Addr) *api.Peer {
	for i := range peers {
		for _, a := range peerAddrs(peers[i]) {
			if a == ip {
				return &peers[i]
			}
		}
	}
	return nil
}

// pingPeer pings ip off the UI goroutine. title is the row's name, shown
// with the result; the GUI log gets only the IP. A ping that opens a
// tunnel without replies is followed by one more.
func (m *mainWindow) pingPeer(ip netip.Addr, title string) {
	if !m.running() || m.peers.pinging[ip] {
		return
	}
	m.peers.setPinging(ip, true)
	m.peers.showResult(ip, title+": "+pingTextPinging, pingWaiting)

	m.spawn(func() {
		before := m.lookupPeer(ip)
		for attempt := 0; ; attempt++ {
			ctx, cancel := context.WithTimeout(m.ctx, pingCallTimeout)
			res, err := m.c.Ping(ctx, api.PingRequest{VpnIP: ip.String()})
			cancel()
			after := m.lookupPeer(ip)
			v := describePing(before, after, res, err)
			m.logPing(ip, res, err, v)
			if v.Retry && attempt == 0 {
				fyne.Do(func() { m.peers.showResult(ip, title+": "+v.Text, v.Kind) })
				before = after
				continue
			}
			fyne.Do(func() { m.pingDone(ip, title, v) })
			return
		}
	})
}

// lookupPeer reads ip's tunnel from the service. An error counts as no
// tunnel: the ping itself reports what's wrong.
func (m *mainWindow) lookupPeer(ip netip.Addr) *api.Peer {
	ctx, cancel := context.WithTimeout(m.ctx, actionTimeout)
	defer cancel()
	r, err := m.c.Peers(ctx)
	if err != nil {
		return nil
	}
	return findPeer(r.Peers, ip)
}

// logPing records a ping in the GUI log by IP only. It's safe to call
// off the UI goroutine.
func (m *mainWindow) logPing(ip netip.Addr, res *api.PingResponse, err error, v pingView) {
	switch {
	case v.Problem != nil:
		m.log.Add("ping failed", "ip", ip.String(), "problem", string(v.Problem.ID), "error", errorDetails(err))
	case err != nil:
		m.log.Add("ping refused", "ip", ip.String(), "error", errorDetails(err))
	default:
		attrs := []string{"ip", ip.String(), "replies", strconv.Itoa(res.Received) + "/" + strconv.Itoa(res.Sent)}
		if res.RTTAvgMs != nil {
			attrs = append(attrs, "avg", formatMs(*res.RTTAvgMs))
		}
		m.log.Add("ping", attrs...)
	}
}

// pingDone shows a finished ping and polls so the row's state and last
// ping time catch up.
func (m *mainWindow) pingDone(ip netip.Addr, title string, v pingView) {
	m.peers.setPinging(ip, false)
	m.peers.showResult(ip, title+": "+v.Text, v.Kind)
	if v.Problem != nil {
		m.showProblemDialog(v.Problem)
	}
	m.poller.Poke()
}

// showProblemDialog shows p with a Get help… button, for problems from
// actions that have no banner of their own.
func (m *mainWindow) showProblemDialog(p *problem) {
	box := container.NewVBox()
	add := func(s string) {
		l := widget.NewLabel(s)
		l.Wrapping = fyne.TextWrapWord
		box.Add(l)
	}
	if p.Explain != "" {
		add(p.Explain)
	}
	for i, s := range p.Steps {
		add(strconv.Itoa(i+1) + ". " + s)
	}
	d := dialog.NewCustomWithoutButtons(p.Title, box, m.w)
	help := widget.NewButton("Get help…", func() {
		d.Hide()
		m.openHelp(p)
	})
	closeB := widget.NewButton("Close", func() { d.Hide() })
	closeB.Importance = widget.HighImportance
	d.SetButtons([]fyne.CanvasObject{help, closeB})
	d.Show()
	d.Resize(fyne.NewSize(problemWidth, d.MinSize().Height))
}
