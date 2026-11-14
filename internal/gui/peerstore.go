// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Saved peer limits. See docs/m3-plan.md#saved-peers.
const (
	maxSavedPeers  = 200
	maxPeerNameLen = 40
)

// peerStoreName is the file's name in the GUI's settings folder.
const peerStoreName = "peers.json"

// savedPeer is a name the user gave to an overlay IP.
type savedPeer struct {
	IP   string `json:"ip"`
	Name string `json:"name"`
}

// addr returns the saved IP, or an invalid Addr if it doesn't parse.
func (p savedPeer) addr() netip.Addr {
	a, err := parsePeerAddr(p.IP)
	if err != nil {
		return netip.Addr{}
	}
	return a
}

type peerFile struct {
	Peers []savedPeer `json:"peers"`
}

var (
	errPeerExists   = errors.New("already in your list")
	errPeerNotSaved = errors.New("not in your list")
	errTooManyPeers = fmt.Errorf("your list can hold at most %d peers", maxSavedPeers)
	errNoPeerName   = errors.New("give it a name")
	errPeerNameLong = fmt.Errorf("names can be at most %d characters", maxPeerNameLen)
	errPeerNameCtrl = errors.New("names can't contain line breaks or tabs")
)

// cleanPeerName trims name and checks it.
func cleanPeerName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", errNoPeerName
	case utf8.RuneCountInString(name) > maxPeerNameLen:
		return "", errPeerNameLong
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "", errPeerNameCtrl
	}
	return name, nil
}

// parsePeerAddr parses an overlay IP as typed by the user or read from
// peers.json. IPv4-mapped IPv6 addresses become IPv4; zones are refused.
func parsePeerAddr(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return netip.Addr{}, err
	}
	if a.Zone() != "" {
		return netip.Addr{}, errors.New("address has a zone")
	}
	return a.Unmap(), nil
}

// peerStore keeps the saved peers in a file readable only by the user.
// With an empty path it keeps them in memory only.
type peerStore struct {
	path string

	mu    sync.Mutex
	peers []savedPeer
}

// defaultPeerStorePath is <user config dir>/nebula-gui/peers.json.
func defaultPeerStorePath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "nebula-gui", peerStoreName), nil
}

// openPeerStore loads the peers saved at path. A missing or damaged file
// starts empty; entries that break the rules (bad IP or name, a repeated
// IP, past the limit) are dropped.
func openPeerStore(path string) *peerStore {
	s := &peerStore{path: path}
	if path == "" {
		return s
	}
	b, err := os.ReadFile(path) //nolint:gosec // the app's own settings file
	if err != nil {
		return s
	}
	var f peerFile
	if json.Unmarshal(b, &f) != nil {
		return s
	}
	seen := map[netip.Addr]bool{}
	for _, p := range f.Peers {
		a, err := parsePeerAddr(p.IP)
		if err != nil || seen[a] {
			continue
		}
		name, err := cleanPeerName(p.Name)
		if err != nil {
			continue
		}
		if len(s.peers) == maxSavedPeers {
			break
		}
		seen[a] = true
		s.peers = append(s.peers, savedPeer{IP: a.String(), Name: name})
	}
	return s
}

// Peers returns a copy of the saved peers, in the order they were added.
func (s *peerStore) Peers() []savedPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]savedPeer(nil), s.peers...)
}

// Add saves a new peer. It fails if ip is already saved.
func (s *peerStore) Add(ip netip.Addr, name string) error {
	name, err := cleanPeerName(name)
	if err != nil {
		return err
	}
	ip = ip.Unmap()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(ip) >= 0 {
		return errPeerExists
	}
	if len(s.peers) >= maxSavedPeers {
		return errTooManyPeers
	}
	next := append(append([]savedPeer(nil), s.peers...), savedPeer{IP: ip.String(), Name: name})
	return s.commit(next)
}

// Rename changes the name of a saved peer.
func (s *peerStore) Rename(ip netip.Addr, name string) error {
	name, err := cleanPeerName(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(ip.Unmap())
	if i < 0 {
		return errPeerNotSaved
	}
	next := append([]savedPeer(nil), s.peers...)
	next[i].Name = name
	return s.commit(next)
}

// Remove forgets a saved peer. Removing one that isn't saved is not an
// error.
func (s *peerStore) Remove(ip netip.Addr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.find(ip.Unmap())
	if i < 0 {
		return nil
	}
	next := append(append([]savedPeer(nil), s.peers[:i]...), s.peers[i+1:]...)
	return s.commit(next)
}

func (s *peerStore) find(ip netip.Addr) int {
	for i, p := range s.peers {
		if p.addr() == ip {
			return i
		}
	}
	return -1
}

// commit saves next and makes it current. If saving fails, nothing
// changes, so the list on screen matches the file.
func (s *peerStore) commit(next []savedPeer) error {
	if s.path != "" {
		b, err := json.MarshalIndent(peerFile{Peers: next}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
			return err
		}
		if err := writePrivateFile(s.path, b); err != nil {
			return err
		}
	}
	s.peers = next
	return nil
}
