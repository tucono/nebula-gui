// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMaskerAddresses(t *testing.T) {
	m := newMasker(true, "", "")
	m.addOverlay("192.168.100.20/24")
	tests := []struct{ in, want string }{
		{"203.0.113.5:4242", "PUBLIC-1:4242"},
		{"again 203.0.113.5", "again PUBLIC-1"},
		{"[2001:db8::1]:4242", "[PUBLIC-2]:4242"},
		{"2001:db8:0:0::1", "PUBLIC-2"}, // the same address, written longer
		{"via fe80::1c2b:3cff:fe4d:5e6f%eth0 now", "via LINKLOCAL-1 now"},
		{"10.1.2.3", "LAN-1"},
		{"100.64.1.1", "LAN-2"},
		{"192.168.100.7", "OVERLAY-1"},
		{"192.168.100.20/24", "OVERLAY-2/24"},
		{"192.168.1.5", "LAN-3"},
		{"fd00::1", "LAN-4"},
		{"::ffff:198.51.100.9", "PUBLIC-3"},
		{"connected to 198.51.100.9.", "connected to PUBLIC-3."},
		{`"udpAddrs":["203.0.113.5:4242"]`, `"udpAddrs":["PUBLIC-1:4242"]`},
		// Kept.
		{"127.0.0.1:4301 0.0.0.0:4242 169.254.1.1 ::1 :: 224.0.0.1", "127.0.0.1:4301 0.0.0.0:4242 169.254.1.1 ::1 :: 224.0.0.1"},
		// Not addresses.
		{"at 14:03:22 on 2026-11-12T14:03:22.114+01:00", "at 14:03:22 on 2026-11-12T14:03:22.114+01:00"},
		{"version 1.12.0, build 1.2.3.4.5, v1.2.3.4", "version 1.12.0, build 1.2.3.4.5, v1.2.3.4"},
		{"mac aa:bb:cc:dd:ee:ff std::string", "mac aa:bb:cc:dd:ee:ff std::string"},
	}
	for _, tt := range tests {
		if got := m.text(tt.in); got != tt.want {
			t.Errorf("text(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMaskerNames(t *testing.T) {
	m := newMasker(true, "/home/tester", "tester")
	m.addHost("dev")
	m.addHost("lighthouse")
	m.addHost("alice")
	m.addHost("alice-laptop")
	m.addHost("lighthouse.example.net")
	m.addGroup("users")
	m.addGroup("gamers")

	// Known fields are replaced whatever the name.
	for in, want := range map[string]string{
		"dev": "HOST-1", "lighthouse": "HOST-2", "Alice": "HOST-3",
		"lighthouse.example.net:4242": "HOST-5:4242", "203.0.113.1": "PUBLIC-1",
		"": "",
	} {
		if in == "lighthouse.example.net:4242" {
			// A name with a port isn't a known field's form; the free-text
			// pass still catches the name.
			if got := m.text(in); got != want {
				t.Errorf("text(%q) = %q, want %q", in, got, want)
			}
			continue
		}
		if got := m.host(in); got != want {
			t.Errorf("host(%q) = %q, want %q", in, got, want)
		}
	}
	if g := m.group("users"); g != "GROUP-1" {
		t.Errorf("group(users) = %q", g)
	}

	tests := []struct{ in, want string }{
		// Short and stop-listed names stay in free text.
		{"open /dev/net/tun: no such device", "open /dev/net/tun: no such device"},
		{"am_lighthouse=false lighthouse.interval=60", "am_lighthouse=false lighthouse.interval=60"},
		{"members of users can ping", "members of users can ping"},
		{"the dev host", "the dev host"},
		// Whole words of collected names go.
		{"alice-laptop connected", "HOST-4 connected"},
		{"alice connected", "HOST-3 connected"},
		{"ALICE connected", "HOST-3 connected"},
		{"seen alice.", "seen HOST-3."},
		{"gamers rule", "GROUP-2 rule"},
		{"resolved lighthouse.example.net", "resolved HOST-5"},
		// So do config file names made from them.
		{"imported alice-laptop.yml and Alice.YAML.", "imported HOST-4.yml and HOST-3.YAML."},
		// Parts of longer words don't.
		{"alice.ymlx alice.example alicea malice alice_x x/alice", "alice.ymlx alice.example alicea malice alice_x x/alice"},
		// The home folder and login name.
		{"/home/tester/nebula/alice.yml", "~/nebula/alice.yml"},
		{"logged in as tester today", "logged in as USER today"},
	}
	for _, tt := range tests {
		if got := m.text(tt.in); got != tt.want {
			t.Errorf("text(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMaskerOff(t *testing.T) {
	m := newMasker(false, "/home/tester", "tester")
	m.addOverlay("192.168.100.0/24")
	m.addHost("alice-laptop")
	m.addGroup("gamers")
	in := "alice-laptop gamers 203.0.113.5:4242 192.168.100.1 /home/tester tester"
	if got := m.text(in); got != in {
		t.Errorf("text = %q", got)
	}
	if m.host("alice-laptop") != "alice-laptop" || m.group("gamers") != "gamers" || m.fingerprint("abcd") != "abcd" {
		t.Error("known fields masked with masking off")
	}
}

func TestMaskerWindowsUser(t *testing.T) {
	m := newMasker(true, "", `HOMEPC\tester`)
	if got := m.text("user tester"); got != "user USER" {
		t.Errorf("text = %q", got)
	}
}

func TestNameStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", nameStoreName)
	s := openNameStore(path)
	if err := s.Remember(rememberedNames{Hosts: []string{"dev"}, Groups: []string{"users"}, Overlays: []string{"192.168.100.0/24"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remember(rememberedNames{Hosts: []string{"DEV", "alice-laptop", " "}}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}

	got := openNameStore(path).Names()
	if !equal(got.Hosts, []string{"dev", "alice-laptop"}) || !equal(got.Groups, []string{"users"}) ||
		!equal(got.Overlays, []string{"192.168.100.0/24"}) {
		t.Fatalf("loaded %+v", got)
	}

	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	if n := s.Names(); len(n.Hosts) != 0 {
		t.Errorf("names after clear: %+v", n)
	}
	if err := s.Clear(); err != nil {
		t.Errorf("clearing twice: %v", err)
	}

	// The oldest entries go once a list is full.
	var r rememberedNames
	for i := range maxRemembered + 5 {
		r.merge(rememberedNames{Hosts: []string{"host" + string(rune('a'+i%26)) + string(rune('a'+i/26))}})
	}
	if len(r.Hosts) != maxRemembered || r.Hosts[0] != "hostfa" {
		t.Errorf("kept %d, first %q", len(r.Hosts), r.Hosts[0])
	}
}

func TestNamesFromConfig(t *testing.T) {
	raw := []byte(`
static_host_map:
  "192.168.100.1": ["lighthouse.example.net:4242", "203.0.113.10:4242"]
  "192.168.100.2": ["[2001:db8::2]:4242", "relay.example.net:4242"]
pki:
  ca: /etc/nebula/ca.crt
`)
	n := namesFromConfig(raw)
	if !equal(n.Hosts, []string{"lighthouse.example.net", "relay.example.net"}) {
		t.Errorf("hosts %v", n.Hosts)
	}
	if n := namesFromConfig([]byte("::: not yaml")); len(n.Hosts) != 0 {
		t.Errorf("from bad yaml: %v", n.Hosts)
	}
}
