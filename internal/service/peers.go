// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"net/netip"
	"slices"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

// peers lists Nebula's established tunnels, then its pending handshakes. A
// host in both is listed once, as established. The list is empty when
// Nebula isn't running.
func (m *manager) peers() []api.Peer {
	// m.mu is held throughout so Stop can't start while Nebula's hostmap
	// is read: stopLocked moves to Stopping under m.mu first.
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []api.Peer{}
	if m.state != api.StateRunning || m.inst == nil {
		return out
	}
	seen := map[netip.Addr]bool{}
	add := func(hosts []nebulactl.HostInfo, state api.PeerState) {
		sortHosts(hosts)
		for _, h := range hosts {
			if len(h.VpnAddrs) == 0 || seen[h.VpnAddrs[0]] {
				continue
			}
			seen[h.VpnAddrs[0]] = true
			out = append(out, toPeer(h, state))
		}
	}
	add(m.inst.Hosts(false), api.PeerEstablished)
	add(m.inst.Hosts(true), api.PeerPending)
	return out
}

// closePeer closes the tunnel to addr. It returns errNotRunning or
// nebulactl.ErrNoTunnel.
func (m *manager) closePeer(addr netip.Addr) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != api.StateRunning || m.inst == nil {
		return errNotRunning
	}
	return m.inst.CloseTunnel(addr)
}

// sortHosts orders hosts by their first overlay address, so the list
// doesn't jump around between calls (Nebula's hostmap is a Go map).
func sortHosts(hosts []nebulactl.HostInfo) {
	slices.SortStableFunc(hosts, func(a, b nebulactl.HostInfo) int {
		switch {
		case len(a.VpnAddrs) == 0 && len(b.VpnAddrs) == 0:
			return 0
		case len(a.VpnAddrs) == 0:
			return 1
		case len(b.VpnAddrs) == 0:
			return -1
		}
		return a.VpnAddrs[0].Compare(b.VpnAddrs[0])
	})
}

func toPeer(h nebulactl.HostInfo, state api.PeerState) api.Peer {
	p := api.Peer{
		VpnIPs:      make([]string, 0, len(h.VpnAddrs)),
		Name:        h.Name,
		Groups:      []string{},
		State:       state,
		RemoteAddrs: make([]string, 0, len(h.RemoteAddrs)),
		Relayed:     h.Relayed,
	}
	for _, a := range h.VpnAddrs {
		p.VpnIPs = append(p.VpnIPs, a.String())
	}
	p.Groups = append(p.Groups, h.Groups...)
	if h.CurrentRemote.IsValid() {
		p.CurrentRemote = h.CurrentRemote.String()
	}
	for _, a := range h.RemoteAddrs {
		p.RemoteAddrs = append(p.RemoteAddrs, a.String())
	}
	if !h.CertNotAfter.IsZero() {
		t := h.CertNotAfter.UTC().Truncate(time.Second)
		p.CertNotAfter = &t
	}
	return p
}
