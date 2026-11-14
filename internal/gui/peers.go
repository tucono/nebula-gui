// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"cmp"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/tucono/nebula-gui/internal/api"
)

// peerStatus is what the Peers tab shows for a row's connection.
type peerStatus int

// The order is the list's sort order.
const (
	peerConnected    peerStatus = iota // Nebula has an established tunnel
	peerConnecting                     // Nebula is handshaking
	peerNotConnected                   // saved, with no tunnel
)

// peerActions says which ⋯ menu items a row has. Copy IP and
// Rename…/Give it a name… are always there.
type peerActions struct {
	Ping   bool // Nebula is running
	Remove bool // the peer is saved
	Close  bool // the tunnel is established
}

// peerRow is one row of the Peers tab.
type peerRow struct {
	// IP is the address actions use: the peer's first overlay IP, or the
	// saved IP for a peer Nebula doesn't know.
	IP netip.Addr
	// Title is the shown name: the saved name, else the cert name, else
	// the IP.
	Title string
	// Subtitle is the IP, plus the cert name when it differs from Title.
	// It's empty when Title is the IP.
	Subtitle  string
	SavedName string // empty when not saved
	CertName  string
	Status    peerStatus
	Relayed   bool
	// LastPingMs is the service's last ping RTT, if any.
	LastPingMs *float64
	Actions    peerActions
}

// Saved reports whether the user gave this peer a name.
func (r peerRow) Saved() bool { return r.SavedName != "" }

// buildPeerRows merges Nebula's peers with the saved ones into the rows of
// the Peers tab. overlays are this host's networks from the latest status;
// saved peers outside them are hidden (kept, but not shown). With no
// overlays known, every saved peer is shown. When Nebula isn't running,
// peers is ignored and only saved peers are listed.
func buildPeerRows(peers []api.Peer, saved []savedPeer, overlays []netip.Prefix, running bool) []peerRow {
	savedBy := map[netip.Addr]savedPeer{}
	for _, s := range saved {
		if a := s.addr(); a.IsValid() {
			savedBy[a] = s
		}
	}
	used := map[netip.Addr]bool{}
	rows := []peerRow{}

	if running {
		for _, p := range peers {
			addrs := peerAddrs(p)
			if len(addrs) == 0 {
				continue
			}
			r := peerRow{IP: addrs[0], CertName: p.Name, Relayed: p.Relayed, LastPingMs: p.LastPingRTTMs}
			switch p.State {
			case api.PeerEstablished:
				r.Status = peerConnected
			default:
				r.Status = peerConnecting
			}
			for _, a := range addrs {
				if s, ok := savedBy[a]; ok && !used[a] {
					r.SavedName = s.Name
					used[a] = true
					break
				}
			}
			rows = append(rows, r)
		}
	}

	for _, s := range saved {
		a := s.addr()
		if !a.IsValid() || used[a] || !inOverlays(a, overlays) {
			continue
		}
		used[a] = true
		rows = append(rows, peerRow{IP: a, SavedName: s.Name, Status: peerNotConnected})
	}

	for i := range rows {
		r := &rows[i]
		r.Title, r.Subtitle = peerTitle(r.SavedName, r.CertName, r.IP)
		r.Actions = peerActions{
			Ping:   running,
			Remove: r.Saved(),
			Close:  r.Status == peerConnected,
		}
	}
	slices.SortStableFunc(rows, func(a, b peerRow) int {
		if c := cmp.Compare(a.Status, b.Status); c != 0 {
			return c
		}
		if c := cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)); c != 0 {
			return c
		}
		return a.IP.Compare(b.IP)
	})
	return rows
}

// peerAddrs parses a peer's overlay IPs, skipping any that don't parse.
func peerAddrs(p api.Peer) []netip.Addr {
	out := make([]netip.Addr, 0, len(p.VpnIPs))
	for _, s := range p.VpnIPs {
		if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, a.Unmap())
		}
	}
	return out
}

// peerTitle picks the shown name and the line below it.
func peerTitle(savedName, certName string, ip netip.Addr) (title, subtitle string) {
	switch {
	case savedName != "":
		if certName != "" && certName != savedName {
			return savedName, ip.String() + " · " + certName
		}
		return savedName, ip.String()
	case certName != "":
		return certName, ip.String()
	default:
		return ip.String(), ""
	}
}

func inOverlays(a netip.Addr, overlays []netip.Prefix) bool {
	if len(overlays) == 0 {
		return true
	}
	for _, p := range overlays {
		if p.Masked().Contains(a) {
			return true
		}
	}
	return false
}

// statusOverlays parses the overlay networks in st, such as
// 192.168.100.20/24. It returns nil when st is nil or has none.
func statusOverlays(st *api.StatusResponse) []netip.Prefix {
	if st == nil {
		return nil
	}
	var out []netip.Prefix
	for _, s := range st.OverlayIPs {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, netip.PrefixFrom(p.Addr().Unmap(), p.Bits()))
		}
	}
	return out
}

// peerInputError is a problem with what the user typed in the Add peer
// dialog. Field is "name" or "ip", so the dialog can show it next to the
// right box.
type peerInputError struct {
	Field string
	Msg   string
}

func (e *peerInputError) Error() string { return e.Msg }

// checkNewPeer checks the Add peer dialog's fields. The IP is checked
// against the networks in st when it has any; otherwise only its format is.
func checkNewPeer(name, ip string, st *api.StatusResponse, saved []savedPeer) error {
	if _, err := cleanPeerName(name); err != nil {
		return &peerInputError{Field: "name", Msg: peerNameMessage(err)}
	}
	overlays := statusOverlays(st)
	a, err := parsePeerAddr(ip)
	if err != nil {
		return &peerInputError{Field: "ip", Msg: "That doesn't look like an address. It should look like " + exampleAddr(overlays) + "."}
	}
	if len(overlays) > 0 {
		for _, p := range overlays {
			if p.Addr() == a {
				return &peerInputError{Field: "ip", Msg: "That's this computer."}
			}
		}
		if !inOverlays(a, overlays) {
			return &peerInputError{Field: "ip", Msg: "That address isn't on your Nebula network. " + describeOverlays(overlays)}
		}
	}
	for _, s := range saved {
		if s.addr() == a {
			return &peerInputError{Field: "ip", Msg: "Already in your list as " + s.Name + "."}
		}
	}
	return nil
}

// exampleAddr returns an address to show as an example: the 10th address
// of the first IPv4 network, or 192.168.100.10 when none is known.
func exampleAddr(overlays []netip.Prefix) string {
	for _, p := range overlays {
		if !p.Addr().Is4() {
			continue
		}
		a := p.Masked().Addr()
		for range 10 {
			a = a.Next()
		}
		if p.Contains(a) {
			return a.String()
		}
		return p.Addr().String()
	}
	if len(overlays) > 0 {
		return overlays[0].Addr().String()
	}
	return "192.168.100.10"
}

// describeOverlays says which addresses are on the network, for example
// "Addresses on your network start with 192.168.100."
func describeOverlays(overlays []netip.Prefix) string {
	parts := make([]string, 0, len(overlays))
	allStart := true
	for _, p := range overlays {
		if s, ok := octetPrefix(p); ok {
			parts = append(parts, s)
		} else {
			allStart = false
			break
		}
	}
	if allStart {
		return "Addresses on your network start with " + strings.Join(parts, " or ") + "."
	}
	parts = parts[:0]
	for _, p := range overlays {
		parts = append(parts, p.Masked().Addr().String()+" to "+lastAddr(p).String())
	}
	return "Addresses on your network are " + strings.Join(parts, ", or ") + "."
}

// octetPrefix returns "192.168.100" for an IPv4 network whose prefix
// length is a whole number of octets (8, 16 or 24).
func octetPrefix(p netip.Prefix) (string, bool) {
	bits := p.Bits()
	if !p.Addr().Is4() || bits%8 != 0 || bits == 0 || bits == 32 {
		return "", false
	}
	b := p.Addr().As4()
	parts := make([]string, 0, bits/8)
	for i := range bits / 8 {
		parts = append(parts, strconv.Itoa(int(b[i])))
	}
	return strings.Join(parts, "."), true
}

// lastAddr returns the last address in p.
func lastAddr(p netip.Prefix) netip.Addr {
	p = p.Masked()
	b := p.Addr().AsSlice()
	for i := range b {
		start := i * 8
		switch {
		case start >= p.Bits():
			b[i] = 0xff
		case start+8 > p.Bits():
			b[i] |= 0xff >> (p.Bits() - start)
		}
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}
