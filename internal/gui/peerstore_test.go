// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestPeerStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", peerStoreName)
	s := openPeerStore(path)
	alice := netip.MustParseAddr("192.168.100.10")
	bob := netip.MustParseAddr("192.168.100.11")

	if err := s.Add(alice, "  Alice's laptop "); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(bob, "Bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(netip.MustParseAddr("::ffff:192.168.100.10"), "Again"); !errors.Is(err, errPeerExists) {
		t.Errorf("adding the same IP again: %v", err)
	}
	if err := s.Rename(bob, "Bob's PC"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(netip.MustParseAddr("192.168.100.99"), "Nobody"); !errors.Is(err, errPeerNotSaved) {
		t.Errorf("renaming an unsaved peer: %v", err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}

	got := openPeerStore(path).Peers()
	want := []savedPeer{{"192.168.100.10", "Alice's laptop"}, {"192.168.100.11", "Bob's PC"}}
	if !slices.Equal(got, want) {
		t.Fatalf("loaded %+v", got)
	}

	if err := s.Remove(alice); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(alice); err != nil {
		t.Errorf("removing twice: %v", err)
	}
	if got := openPeerStore(path).Peers(); !slices.Equal(got, want[1:]) {
		t.Errorf("after remove %+v", got)
	}
}

func TestPeerStoreNames(t *testing.T) {
	s := openPeerStore("")
	ip := netip.MustParseAddr("192.168.100.10")
	for _, tc := range []struct {
		name string
		want error
	}{
		{"", errNoPeerName},
		{"   ", errNoPeerName},
		{strings.Repeat("é", maxPeerNameLen+1), errPeerNameLong},
		{"two\nlines", errPeerNameCtrl},
		{"tab\there", errPeerNameCtrl},
	} {
		if err := s.Add(ip, tc.name); !errors.Is(err, tc.want) {
			t.Errorf("Add(%q) = %v, want %v", tc.name, err, tc.want)
		}
	}
	// 40 characters, more than 40 bytes.
	if err := s.Add(ip, strings.Repeat("é", maxPeerNameLen)); err != nil {
		t.Errorf("40 characters: %v", err)
	}
	if err := s.Rename(ip, ""); !errors.Is(err, errNoPeerName) {
		t.Errorf("Rename to empty: %v", err)
	}
}

func TestPeerStoreLimit(t *testing.T) {
	s := openPeerStore("")
	for i := range maxSavedPeers {
		a := netip.AddrFrom4([4]byte{10, 0, 0, 0}).Next()
		for range i {
			a = a.Next()
		}
		if err := s.Add(a, "p"+strconv.Itoa(i)); err != nil {
			t.Fatalf("peer %d: %v", i, err)
		}
	}
	if err := s.Add(netip.MustParseAddr("10.0.1.0"), "one more"); !errors.Is(err, errTooManyPeers) {
		t.Errorf("past the limit: %v", err)
	}
}

func TestPeerStoreDamagedFile(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"not json":   "{",
		"wrong type": `{"peers": "x"}`,
	} {
		path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_"))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := openPeerStore(path).Peers(); len(got) != 0 {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := openPeerStore(filepath.Join(dir, "missing.json")).Peers(); len(got) != 0 {
		t.Errorf("missing file: %+v", got)
	}
}

func TestPeerStoreDropsBadEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), peerStoreName)
	content := `{"peers": [
		{"ip": "192.168.100.10", "name": " Alice "},
		{"ip": "192.168.100.10", "name": "Duplicate"},
		{"ip": "not an ip", "name": "Bad IP"},
		{"ip": "fe80::1%eth0", "name": "Zone"},
		{"ip": "192.168.100.11", "name": ""},
		{"ip": "::ffff:192.168.100.12", "name": "Mapped"}
	]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := openPeerStore(path).Peers()
	want := []savedPeer{{"192.168.100.10", "Alice"}, {"192.168.100.12", "Mapped"}}
	if !slices.Equal(got, want) {
		t.Errorf("loaded %+v", got)
	}
}

func TestPeerStoreSaveFailureKeepsList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a file standing where the folder should be")
	}
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := openPeerStore(filepath.Join(blocker, peerStoreName))
	if err := s.Add(netip.MustParseAddr("192.168.100.10"), "Alice"); err == nil {
		t.Fatal("expected an error")
	}
	if got := s.Peers(); len(got) != 0 {
		t.Errorf("peer kept after a failed save: %+v", got)
	}
}
