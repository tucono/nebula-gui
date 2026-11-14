// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// masker replaces private network details in a troubleshooting report with
// placeholders such as PUBLIC-1 and HOST-2. One masker is used for a whole
// report, so the same address or name gets the same placeholder in every
// section. See "Masking" in docs/m2-plan.md.
//
// Names are collected first (addHost, addGroup, addFingerprint, from the
// status, config summary, remembered names and known log fields). Known
// fields are then replaced by value; free text only loses whole words of at
// least minFreeTextName characters that aren't on the stop list, so a host
// called dev doesn't turn /dev/net/tun into /HOST-1/net/tun. Every IP
// address is masked by range, whether or not it was collected.
//
// With masking off, every method returns its input unchanged.
type masker struct {
	on       bool
	home     string
	user     string
	overlays []netip.Prefix

	// names maps a lower-case name to its placeholder.
	names map[string]string
	// addrs maps an address, in netip's form, to its placeholder.
	addrs map[string]string
	// counts holds the last number given out per placeholder kind.
	counts map[string]int
	// words matches the free-text names; nil until needed, and reset
	// whenever a name is added.
	words *regexp.Regexp
}

// Placeholder kinds.
const (
	maskPublic    = "PUBLIC"
	maskLAN       = "LAN"
	maskOverlay   = "OVERLAY"
	maskLinkLocal = "LINKLOCAL"
	maskHost      = "HOST"
	maskGroup     = "GROUP"
	maskFP        = "FP"
	maskUser      = "USER"
)

// minFreeTextName is the shortest name replaced in free text. Shorter
// names are still replaced in known fields.
const minFreeTextName = 4

// maskStopWords are never replaced in free text, because Nebula and the OS
// use them in their own messages. They're still replaced in known fields.
var maskStopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		lighthouse lighthouses relay relays dev user users admin admins host
		hosts nebula node nodes server servers client clients local localhost
		default root home guest test tests any all none null true false group
		groups network networks config cert certs certificate tun tun0 tap
		linux windows macos darwin debug info warn warning error fatal panic
		trace main public private internal service daemon system laptop
		desktop phone mobile handshake tunnel firewall inbound outbound
		incoming outgoing direct punch punchy listen port remote static
		member members everyone family friends staff work home office`) {
		maskStopWords[w] = true
	}
}

// newMasker returns a masker; with on false it changes nothing. home and
// user are the user's home folder and login name, or empty if unknown.
func newMasker(on bool, home, user string) *masker {
	if i := strings.LastIndexAny(user, `\/`); i >= 0 {
		user = user[i+1:] // DOMAIN\user on Windows
	}
	return &masker{
		on:     on,
		home:   home,
		user:   user,
		names:  map[string]string{},
		addrs:  map[string]string{},
		counts: map[string]int{},
	}
}

// addOverlay records an overlay network. s is a prefix ("192.168.100.20/24",
// a host's address with its network's length) or a bare address; anything
// else is ignored.
func (m *masker) addOverlay(s string) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil {
		return
	}
	p = p.Masked()
	for _, o := range m.overlays {
		if o == p {
			return
		}
	}
	m.overlays = append(m.overlays, p)
}

func (m *masker) addHost(name string)        { m.addName(maskHost, name) }
func (m *masker) addGroup(name string)       { m.addName(maskGroup, name) }
func (m *masker) addFingerprint(name string) { m.addName(maskFP, name) }

// addName gives name a placeholder of kind, unless it has one already.
func (m *masker) addName(kind, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	key := strings.ToLower(name)
	if p, ok := m.names[key]; ok {
		return p
	}
	// A name that is an address is masked as an address.
	if _, err := netip.ParseAddr(name); err == nil {
		return ""
	}
	p := m.next(kind)
	m.names[key] = p
	m.words = nil
	return p
}

func (m *masker) next(kind string) string {
	m.counts[kind]++
	return fmt.Sprintf("%s-%d", kind, m.counts[kind])
}

// host masks a value from a field known to hold a host name.
func (m *masker) host(name string) string { return m.known(maskHost, name) }

// group masks a value from a field known to hold a group.
func (m *masker) group(name string) string { return m.known(maskGroup, name) }

// fingerprint masks a value from a field known to hold a fingerprint.
func (m *masker) fingerprint(fp string) string { return m.known(maskFP, fp) }

func (m *masker) known(kind, s string) string {
	if !m.on || strings.TrimSpace(s) == "" {
		return s
	}
	if _, _, err := net.SplitHostPort(s); err == nil || isAddr(s) {
		return m.text(s) // an address, or a name with a port
	}
	if p := m.addName(kind, s); p != "" {
		return p
	}
	return m.text(s)
}

func isAddr(s string) bool {
	_, err := netip.ParseAddr(s)
	return err == nil
}

// text masks free text: the home folder, then names, then addresses.
func (m *masker) text(s string) string {
	if !m.on || s == "" {
		return s
	}
	s = homeToTilde(s, m.home)
	s = m.replaceNames(s)
	return m.replaceAddrs(s)
}

// replaceNames replaces the collected names, and the user's login name,
// where they stand as whole words.
func (m *masker) replaceNames(s string) string {
	re := m.wordsRegexp()
	if re == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range re.FindAllStringIndex(s, -1) {
		i, j := loc[0], loc[1]
		if !wordEdge(s, i-1) || !wordEnd(s, j) {
			continue
		}
		w := strings.ToLower(s[i:j])
		p, ok := m.names[w]
		if !ok {
			p = maskUser
		}
		b.WriteString(s[last:i])
		b.WriteString(p)
		last = j
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// wordsRegexp matches every name long enough and not on the stop list,
// longest first, so alice-laptop wins over alice.
func (m *masker) wordsRegexp() *regexp.Regexp {
	if m.words != nil {
		return m.words
	}
	var words []string
	for w := range m.names {
		if freeTextName(w) {
			words = append(words, w)
		}
	}
	if u := strings.ToLower(m.user); freeTextName(u) {
		if _, ok := m.names[u]; !ok {
			words = append(words, u)
		}
	}
	if len(words) == 0 {
		return nil
	}
	sort.Slice(words, func(i, j int) bool {
		if len(words[i]) != len(words[j]) {
			return len(words[i]) > len(words[j])
		}
		return words[i] < words[j]
	})
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	m.words = regexp.MustCompile(`(?i)(?:` + strings.Join(words, "|") + `)`)
	return m.words
}

func freeTextName(w string) bool {
	return len(w) >= minFreeTextName && !maskStopWords[w]
}

// wordEdge reports whether s[i] can stand next to a whole word: the ends of
// s, or any byte that isn't a letter, digit, /, _, . or -.
func wordEdge(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return false
	case c == '/', c == '_', c == '.', c == '-', c >= 0x80:
		return false
	default:
		return true
	}
}

// addrCandidate finds text that might be an IPv6 address (with an optional
// zone) or an IPv4 address. Each match is checked with netip.ParseAddr, so
// times such as 14:03:22 and versions such as 1.12.0 are left alone.
var addrCandidate = regexp.MustCompile(
	`(?i)(?:[0-9a-f]{0,4}:){2,7}(?:\d{1,3}(?:\.\d{1,3}){3}|[0-9a-f]{1,4})?(?:%[0-9a-z_.-]+)?` +
		`|\d{1,3}(?:\.\d{1,3}){3}`)

// replaceAddrs masks every IP address in s by its range.
func (m *masker) replaceAddrs(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range addrCandidate.FindAllStringIndex(s, -1) {
		i, j := loc[0], loc[1]
		if !addrEdge(s, i-1) || !addrEnd(s, j) {
			continue
		}
		cand := s[i:j]
		a, err := netip.ParseAddr(cand)
		if err != nil {
			continue
		}
		p := m.addr(a)
		if p == "" {
			continue
		}
		b.WriteString(s[last:i])
		b.WriteString(p)
		last = j
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// fileExts are the extensions after which a name still counts as a whole
// word, so a config called alice-laptop.yml loses the host's name.
var fileExts = []string{".yml", ".yaml", ".conf", ".txt", ".crt", ".key", ".pem", ".md", ".zip"}

// fileExtAt reports whether s continues at i with a file extension that
// ends a word.
func fileExtAt(s string, i int) bool {
	for _, e := range fileExts {
		if len(s)-i >= len(e) && strings.EqualFold(s[i:i+len(e)], e) && (wordEdge(s, i+len(e)) || sentenceEnd(s, i+len(e))) {
			return true
		}
	}
	return false
}

// sentenceEnd reports whether s[i] is a full stop that ends a sentence
// rather than continuing a name or address: it's followed by the end of s
// or by a byte that isn't a letter or digit.
func sentenceEnd(s string, i int) bool {
	if i >= len(s) || s[i] != '.' {
		return false
	}
	if i+1 == len(s) {
		return true
	}
	c := s[i+1]
	return !alnum(c) && c < 0x80
}

func alnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// wordEnd reports whether a name can end just before s[i]: a word edge,
// a full stop ending a sentence, or a file extension.
func wordEnd(s string, i int) bool {
	return wordEdge(s, i) || sentenceEnd(s, i) || fileExtAt(s, i)
}

// addrEnd reports whether an address can end just before s[i].
func addrEnd(s string, i int) bool {
	return addrEdge(s, i) || sentenceEnd(s, i)
}

// addrEdge reports whether s[i] can stand next to an address: not a letter,
// digit, '.' or '_', so 1.2.3.4.5 and abc1.2.3.4 aren't taken apart. A ':'
// is allowed, since a port can follow an address.
func addrEdge(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return false
	case c == '.', c == '_':
		return false
	default:
		return true
	}
}

// addr returns the placeholder for a, or "" for addresses that are kept:
// loopback, unspecified, IPv4 link-local and multicast. Overlay is checked
// before the LAN ranges, since overlays often use private space.
func (m *masker) addr(a netip.Addr) string {
	a = a.Unmap()
	plain := a.WithZone("")
	switch {
	case plain.IsLoopback(), plain.IsUnspecified(), plain.IsMulticast(),
		plain.Is4() && plain.IsLinkLocalUnicast(),
		plain == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		return ""
	}
	key := plain.String()
	if p, ok := m.addrs[key]; ok {
		return p
	}
	var kind string
	switch {
	case m.inOverlay(plain):
		kind = maskOverlay
	case plain.IsLinkLocalUnicast():
		kind = maskLinkLocal
	case plain.IsPrivate() || cgnat.Contains(plain):
		kind = maskLAN
	default:
		kind = maskPublic
	}
	p := m.next(kind)
	m.addrs[key] = p
	return p
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func (m *masker) inOverlay(a netip.Addr) bool {
	for _, o := range m.overlays {
		if o.Contains(a) {
			return true
		}
	}
	return false
}

// rememberedNames are the names the GUI keeps for masking, so a report
// made while the service can't be reached still masks the GUI log. They
// come from the status and the config, and are cleared when the config is
// removed.
type rememberedNames struct {
	Hosts       []string `json:"hosts,omitempty"`
	Groups      []string `json:"groups,omitempty"`
	Overlays    []string `json:"overlays,omitempty"`
	Fingerprint []string `json:"fingerprints,omitempty"`
}

// maxRemembered is how many entries each list keeps; the oldest go first.
const maxRemembered = 100

// merge adds the entries of o that aren't there yet, and reports whether
// anything was added.
func (r *rememberedNames) merge(o rememberedNames) bool {
	changed := false
	add := func(list *[]string, vals []string) {
		for _, v := range vals {
			v = strings.TrimSpace(v)
			if v == "" || containsFold(*list, v) {
				continue
			}
			*list = append(*list, v)
			changed = true
		}
		if n := len(*list) - maxRemembered; n > 0 {
			*list = append([]string(nil), (*list)[n:]...)
		}
	}
	add(&r.Hosts, o.Hosts)
	add(&r.Groups, o.Groups)
	add(&r.Overlays, o.Overlays)
	add(&r.Fingerprint, o.Fingerprint)
	return changed
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// nameStore keeps the remembered names in a file readable only by the
// user. With an empty path it keeps them in memory only.
type nameStore struct {
	path string

	mu    sync.Mutex
	names rememberedNames
}

// nameStoreName is the file's name in the GUI's settings folder.
const nameStoreName = "mask-names.json"

// defaultNameStorePath is <user config dir>/nebula-gui/mask-names.json.
func defaultNameStorePath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "nebula-gui", nameStoreName), nil
}

// openNameStore loads the names saved at path. A missing or damaged file
// starts empty.
func openNameStore(path string) *nameStore {
	s := &nameStore{path: path}
	if path == "" {
		return s
	}
	b, err := os.ReadFile(path) //nolint:gosec // the app's own settings file
	if err == nil {
		_ = json.Unmarshal(b, &s.names)
	}
	return s
}

// Names returns a copy of the remembered names.
func (s *nameStore) Names() rememberedNames {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.names
	n.Hosts = append([]string(nil), n.Hosts...)
	n.Groups = append([]string(nil), n.Groups...)
	n.Overlays = append([]string(nil), n.Overlays...)
	n.Fingerprint = append([]string(nil), n.Fingerprint...)
	return n
}

// Remember adds names and saves the file if anything is new.
func (s *nameStore) Remember(n rememberedNames) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.names.merge(n) {
		return nil
	}
	return s.save()
}

// Clear forgets every name and removes the file.
func (s *nameStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = rememberedNames{}
	if s.path == "" {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *nameStore) save() error {
	if s.path == "" {
		return nil
	}
	b, err := json.Marshal(s.names)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return writePrivateFile(s.path, b)
}

// writePrivateFile writes data to path readable only by the user, also when
// the file was already there with other permissions.
func writePrivateFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
