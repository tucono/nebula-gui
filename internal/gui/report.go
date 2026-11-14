// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	// reportMaxBytes is about the most a chat message takes, including on
	// free AI chat tiers. The oldest service log lines are left out first.
	reportMaxBytes = 50 << 10
	// reportEvents is how many GUI log entries the report includes.
	reportEvents = 100
	// reportTimeFormat is the full time used for GUI log entries.
	reportTimeFormat = "2006-01-02T15:04:05.000Z07:00"
)

// errReportUnsafe is returned when the finished report still contains key
// material. Inputs with key material are dropped before that, so this
// means a bug elsewhere; the report isn't shown rather than risk it.
var errReportUnsafe = errors.New("the report contains key material and wasn't created; please report this bug")

// reportInput is everything a troubleshooting report is made from. See
// "Report format" in docs/m2-plan.md.
type reportInput struct {
	Now          time.Time
	AppVersion   string
	GOOS, GOARCH string
	// Version is the service's last version response, for when the
	// diagnostics can't be read; nil if it was never reached.
	Version *api.VersionResponse
	// Problem is the problem the user asked for help with; nil when the
	// report was opened without one.
	Problem *problem
	// Status is the last status seen, used when Diag is nil.
	Status *api.StatusResponse
	// Diag is the service's diagnostics, or nil with DiagErr saying why.
	Diag    *api.DiagnosticsResponse
	DiagErr error
	// Events are the newest GUI log entries, oldest first.
	Events []appEvent
	// Names are the names remembered for masking.
	Names rememberedNames
	// Mask hides private network details.
	Mask bool
	// Home and User are the user's home folder and login name, masked as
	// ~ and USER.
	Home, User string
}

// buildReport writes the troubleshooting report as Markdown. It fails only
// when the finished text still contains key material.
func buildReport(in reportInput) (string, error) {
	r := &reporter{in: in, m: newMasker(in.Mask, in.Home, in.User)}
	r.status = in.Status
	if in.Diag != nil {
		r.status = &in.Diag.Status
	}
	r.prepare()

	text := r.render(r.lines, 0)
	if len(text) > reportMaxBytes && len(r.lines) > 0 {
		// Keep the newest lines that fit next to everything else. The
		// note about the dropped lines is measured with the most digits it
		// can have.
		base := len(r.render(nil, len(r.lines)))
		// The fence around the lines isn't in base; 64 bytes covers it.
		budget := reportMaxBytes - base - 64
		keep := 0
		for i := len(r.lines) - 1; i >= 0; i-- {
			if budget -= len(r.lines[i]) + 1; budget < 0 {
				break
			}
			keep++
		}
		dropped := len(r.lines) - keep
		text = r.render(r.lines[dropped:], dropped)
	}
	if unsafeText(text) {
		return "", errReportUnsafe
	}
	return text, nil
}

// unsafeText reports whether s holds key material. Every input line that
// does is dropped from the report.
func unsafeText(s string) bool {
	return strings.Contains(s, "PRIVATE KEY") || strings.Contains(s, "BEGIN NEBULA")
}

// reporter builds one report.
type reporter struct {
	in     reportInput
	m      *masker
	status *api.StatusResponse

	// Filled in by prepare.
	problemTitle   string
	problemDetails string
	state          []string
	events         []string
	// lines are the service log lines, masked and shortened.
	lines []string
	// removed counts input lines dropped for holding key material.
	removed int
}

// prepare collects the names to mask, in a fixed order so placeholders
// are numbered the same way every time, and then masks each section in
// the order the report shows them.
func (r *reporter) prepare() {
	m := r.m

	if st := r.status; st != nil {
		for _, ip := range st.OverlayIPs {
			m.addOverlay(ip)
		}
		if c := st.Cert; c != nil {
			m.addHost(c.Name)
			for _, g := range c.Groups {
				m.addGroup(g)
			}
			m.addFingerprint(c.Fingerprint)
		}
	}
	if sum := r.summary(); sum != nil {
		for _, lh := range sum.Lighthouses {
			if h := hostOnly(lh); !isAddr(h) {
				m.addHost(h)
			}
		}
		for _, h := range sum.StaticHostNames {
			m.addHost(h)
		}
	}
	n := r.in.Names
	for _, o := range n.Overlays {
		m.addOverlay(o)
	}
	for _, h := range n.Hosts {
		m.addHost(h)
	}
	for _, g := range n.Groups {
		m.addGroup(g)
	}
	for _, fp := range n.Fingerprint {
		m.addFingerprint(fp)
	}

	// Parse the service log and collect the names in its known fields.
	type logLine struct {
		fields map[string]any
		raw    string
	}
	var parsed []logLine
	if r.in.Diag != nil {
		for _, raw := range r.in.Diag.Log {
			if unsafeText(raw) {
				r.removed++
				continue
			}
			var f map[string]any
			d := json.NewDecoder(strings.NewReader(raw))
			d.UseNumber()
			if d.Decode(&f) != nil {
				f = nil
			}
			collectNames(m, "", f)
			parsed = append(parsed, logLine{fields: f, raw: raw})
		}
	}

	// Mask in the order the report shows the sections.
	if p := r.in.Problem; p != nil {
		r.problemTitle = fmt.Sprintf("%s [%s]", m.text(p.Title), p.ID)
		if unsafeText(p.Details) {
			r.removed++
		} else {
			r.problemDetails = m.text(p.Details)
		}
	}
	r.state = r.stateLines()
	for _, e := range r.in.Events {
		if eventUnsafe(e) {
			r.removed++
			continue
		}
		r.events = append(r.events, r.event(e))
	}
	for _, l := range parsed {
		if l.fields == nil {
			r.lines = append(r.lines, m.text(l.raw))
			continue
		}
		r.lines = append(r.lines, r.logLine(l.fields))
	}
}

func (r *reporter) summary() *api.ConfigSummary {
	if r.in.Diag == nil {
		return nil
	}
	return r.in.Diag.ConfigSummary
}

func eventUnsafe(e appEvent) bool {
	if unsafeText(e.Msg) {
		return true
	}
	for k, v := range e.Attrs {
		if unsafeText(k) || unsafeText(v) {
			return true
		}
	}
	return false
}

// render writes the report with the given service log lines; dropped is
// how many older lines were left out for size.
func (r *reporter) render(lines []string, dropped int) string {
	in := r.in
	var b strings.Builder
	b.WriteString("# Nebula troubleshooting report\n")
	fmt.Fprintf(&b, "Generated %s · nebula-gui %s · service %s · Nebula %s · %s\n",
		in.Now.Format(time.RFC3339), orUnknown(in.AppVersion),
		orUnknown(r.serviceVersion()), orUnknown(r.nebulaVersion()), r.osLine())
	if in.Mask {
		b.WriteString("Masking: on\n")
	} else {
		b.WriteString("Masking: off (network details included)\n")
	}
	if r.removed > 0 {
		fmt.Fprintf(&b, "%d %s removed for safety\n", r.removed, pick(r.removed, "line", "lines"))
	}

	if r.problemTitle != "" {
		b.WriteString("\n## Problem\n")
		b.WriteString(r.problemTitle + "\n")
		if r.problemDetails != "" {
			b.WriteString(r.problemDetails + "\n")
		}
	}

	b.WriteString("\n## State\n")
	for _, l := range r.state {
		b.WriteString(l + "\n")
	}

	b.WriteString("\n## Recent app events\n")
	if len(r.events) == 0 {
		b.WriteString("None recorded.\n")
	} else {
		writeBlock(&b, "", r.events)
	}

	if in.Diag == nil {
		b.WriteString("\n## Service log\n")
		b.WriteString("Not available: the background service couldn't be reached.\n")
	} else {
		fmt.Fprintf(&b, "\n## Service log (last %d %s, JSON Lines)\n", len(lines), pick(len(lines), "line", "lines"))
		if dropped > 0 {
			fmt.Fprintf(&b, "The oldest %d %s were left out to keep this report under %d KB.\n",
				dropped, pick(dropped, "line", "lines"), reportMaxBytes>>10)
		}
		if len(lines) == 0 {
			b.WriteString("Nothing logged.\n")
		} else {
			writeBlock(&b, "jsonl", lines)
		}
	}

	b.WriteString("\n## Notes for the assistant\n")
	for _, n := range r.notes() {
		b.WriteString("- " + n + "\n")
	}
	return b.String()
}

// writeBlock writes lines as a fenced code block, so a chat shows them as
// written.
func writeBlock(b *strings.Builder, lang string, lines []string) {
	b.WriteString("```" + lang + "\n")
	for _, l := range lines {
		b.WriteString(strings.ReplaceAll(l, "```", "'''") + "\n")
	}
	b.WriteString("```\n")
}

func (r *reporter) notes() []string {
	n := []string{
		"This is a Nebula overlay network client. nebula-guid is a privileged background " +
			"service that runs Nebula; nebula-gui is the desktop app the user sees.",
		"The user is not technical. Suggest one step at a time and say where to click.",
	}
	if r.in.Mask {
		n = append(n, "Names and addresses are masked consistently: PUBLIC-1 is always the same address. "+
			"PUBLIC is an internet address, LAN a local network address, OVERLAY an address on the Nebula network, "+
			"HOST a host or certificate name, GROUP a certificate group and FP a certificate fingerprint.")
	} else {
		n = append(n, "Masking is off: names and addresses are real.")
	}
	n = append(n, "Only the person who runs the network can issue new certificates or configs.")
	if r.in.Diag == nil {
		if r.in.GOOS == "linux" {
			n = append(n, "The service log isn't available. On Linux, `journalctl -u nebula-guid -n 200` shows it.")
		} else {
			n = append(n, "The service log isn't available. The service writes to its log first thing when it starts, "+
				"so once it runs again a new report includes why it stopped.")
		}
	}
	return n
}

func (r *reporter) serviceVersion() string {
	if r.in.Diag != nil {
		return r.in.Diag.ServiceVersion
	}
	if r.in.Version != nil {
		return r.in.Version.ServiceVersion
	}
	return ""
}

func (r *reporter) nebulaVersion() string {
	if r.in.Diag != nil {
		return r.in.Diag.NebulaVersion
	}
	if r.in.Version != nil {
		return r.in.Version.NebulaVersion
	}
	return ""
}

func (r *reporter) osLine() string {
	goos, arch, ver := r.in.GOOS, r.in.GOARCH, ""
	if d := r.in.Diag; d != nil {
		goos, arch, ver = d.OS, d.Arch, d.OSVersion
	}
	s := osName(goos) + " " + arch
	if ver != "" {
		s += " (" + ver + ")"
	}
	return s
}

func osName(goos string) string {
	switch goos {
	case "linux":
		return "Linux"
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	default:
		return orUnknown(goos)
	}
}

// stateLines is the State section: the status, then the config summary.
func (r *reporter) stateLines() []string {
	m := r.m
	st := r.status
	if st == nil {
		why := "unknown error"
		if r.in.DiagErr != nil {
			why = m.text(errorDetails(r.in.DiagErr))
		}
		return []string{"The background service couldn't be reached: " + why}
	}

	var out []string
	parts := []string{"state=" + string(st.State), "configured=" + yesNo(st.Configured)}
	if len(st.OverlayIPs) > 0 {
		ips := make([]string, len(st.OverlayIPs))
		for i, ip := range st.OverlayIPs {
			ips[i] = m.text(ip)
		}
		parts = append(parts, "overlay "+strings.Join(ips, ", "))
	}
	if st.StartedAt != nil {
		parts = append(parts, "connected since "+st.StartedAt.Format(time.RFC3339))
	}
	out = append(out, strings.Join(parts, " · "))
	if st.Error != "" || st.ErrorReason != "" {
		e := "error"
		if st.ErrorReason != "" {
			e += " (" + string(st.ErrorReason) + ")"
		}
		out = append(out, e+": "+m.text(st.Error))
	}

	if c := st.Cert; c != nil {
		var groups []string
		for _, g := range c.Groups {
			groups = append(groups, m.group(g))
		}
		line := "cert " + m.host(c.Name)
		if len(groups) > 0 {
			line += ", groups " + strings.Join(groups, ", ")
		}
		line += ", valid until " + c.NotAfter.Format(time.DateOnly)
		if c.Expired {
			line += " (expired)"
		}
		if c.CANotAfter != nil {
			line += ", CA valid until " + c.CANotAfter.Format(time.DateOnly)
			if c.CAExpired {
				line += " (expired)"
			}
		}
		if st.ListenPort != 0 {
			line += fmt.Sprintf(" · listen port %d", st.ListenPort)
		}
		out = append(out, line)
	}

	switch sum := r.summary(); {
	case sum != nil:
		out = append(out, r.configLine(sum))
	case r.in.Diag == nil:
		why := "unknown error"
		if r.in.DiagErr != nil {
			why = m.text(errorDetails(r.in.DiagErr))
		}
		out = append(out, "Config summary not available: "+why)
	case !st.Configured:
		out = append(out, "Config: none stored")
	default:
		out = append(out, "Config: stored, but it couldn't be summarized")
	}
	return out
}

func (r *reporter) configLine(sum *api.ConfigSummary) string {
	lh := make([]string, len(sum.Lighthouses))
	for i, a := range sum.Lighthouses {
		lh[i] = r.hostPort(a)
	}
	s := fmt.Sprintf("Config: %d %s", len(lh), pick(len(lh), "lighthouse", "lighthouses"))
	if len(lh) > 0 {
		s += " (" + strings.Join(lh, ", ") + ")"
	}
	s += fmt.Sprintf(", am_lighthouse=%t, relays %s, punchy %s", sum.AmLighthouse, onOff(sum.Relays), onOff(sum.Punchy))
	if sum.ListenPort != 0 {
		s += fmt.Sprintf(", listen port %d", sum.ListenPort)
	}
	s += fmt.Sprintf(",\n%d tun.routes, %d unsafe_routes, firewall %d inbound / %d outbound rules",
		sum.TunRoutes, sum.UnsafeRoutes, sum.FirewallInbound, sum.FirewallOutbound)
	return s
}

// hostPort masks an underlay address that may be a DNS name with a port.
func (r *reporter) hostPort(s string) string {
	h, port, err := net.SplitHostPort(s)
	if err != nil {
		h, port = s, ""
	}
	if isAddr(h) {
		return r.m.text(s)
	}
	out := r.m.host(h)
	if port != "" {
		out += ":" + port
	}
	return out
}

// event is one GUI log entry: time, message and attributes, sorted.
func (r *reporter) event(e appEvent) string {
	var b strings.Builder
	b.WriteString(e.Time.Format(reportTimeFormat))
	b.WriteByte(' ')
	b.WriteString(r.m.text(e.Msg))
	keys := make([]string, 0, len(e.Attrs))
	for k := range e.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%s", k, quoteIfNeeded(r.m.text(e.Attrs[k])))
	}
	return b.String()
}

// Log field keys whose values are names; see "Masking" in docs/m2-plan.md.
// Add new Nebula fields that carry names here.
var (
	hostKeys = map[string]bool{
		"certName": true, "caName": true, "hostname": true, "hostName": true,
		"remoteCertName": true,
	}
	groupKeys = map[string]bool{"groups": true, "group": true}
	fpKeys    = map[string]bool{
		"fingerprint": true, "fingerprints": true, "issuer": true,
		"caSha": true, "caFingerprint": true,
	}
	// overlayKeys hold this host's or a peer's overlay networks.
	overlayKeys = map[string]bool{"networks": true, "ips": true, "vpnNetworks": true}
	// dropKeys are left out of log lines: long, and of no use to a reader.
	dropKeys = map[string]bool{"publicKey": true, "signature": true}
	// plainKeys are written as they are.
	plainKeys = map[string]bool{"time": true, "level": true}
)

// collectNames adds the names in a log line's known fields to m. key is
// the key v is stored under.
func collectNames(m *masker, key string, v any) {
	switch v := v.(type) {
	case string:
		switch {
		case hostKeys[key]:
			if h := hostOnly(v); !isAddr(h) {
				m.addHost(h)
			}
		case groupKeys[key]:
			m.addGroup(v)
		case fpKeys[key]:
			m.addFingerprint(v)
		case overlayKeys[key]:
			m.addOverlay(v)
		}
	case []any:
		for _, e := range v {
			collectNames(m, key, e)
		}
	case map[string]any:
		if isCertObject(v) {
			d, _ := v["details"].(map[string]any)
			collectNames(m, "certName", d["name"])
			collectNames(m, "groups", d["groups"])
			collectNames(m, "networks", d["networks"])
			collectNames(m, "networks", d["ips"])
			collectNames(m, "issuer", d["issuer"])
			collectNames(m, "fingerprint", v["fingerprint"])
			return
		}
		for _, k := range sortedKeys(v) {
			collectNames(m, k, v[k])
		}
	}
}

// hostOnly drops a port from host:port.
func hostOnly(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

// logLine writes a service log line as JSON with time, level, msg and
// component first and the other keys sorted, masked and with certificates
// shortened.
func (r *reporter) logLine(f map[string]any) string {
	var b bytes.Buffer
	b.WriteByte('{')
	first := true
	write := func(k string) {
		v, ok := f[k]
		if !ok || dropKeys[k] {
			return
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		writeJSON(&b, k)
		b.WriteByte(':')
		writeJSON(&b, r.value(k, v))
	}
	lead := []string{"time", "level", "msg", "component"}
	for _, k := range lead {
		write(k)
	}
	for _, k := range sortedKeys(f) {
		switch k {
		case "time", "level", "msg", "component":
		default:
			write(k)
		}
	}
	b.WriteByte('}')
	return b.String()
}

// value masks v, stored under key.
func (r *reporter) value(key string, v any) any {
	m := r.m
	switch v := v.(type) {
	case string:
		switch {
		case plainKeys[key]:
			return v
		case hostKeys[key]:
			return r.hostPort(v)
		case groupKeys[key]:
			return m.group(v)
		case fpKeys[key]:
			return m.fingerprint(v)
		default:
			return m.text(v)
		}
	case []any:
		out := make([]any, len(v))
		for i, e := range v {
			out[i] = r.value(key, e)
		}
		if key == "cert" && len(out) == 1 {
			return out[0]
		}
		return out
	case map[string]any:
		if isCertObject(v) {
			return r.shortCert(v)
		}
		out := make(map[string]any, len(v))
		for k, e := range v {
			if !dropKeys[k] {
				out[k] = r.value(k, e)
			}
		}
		return out
	default:
		return v
	}
}

// isCertObject reports whether v is a certificate as Nebula logs it.
func isCertObject(v map[string]any) bool {
	_, ok := v["details"].(map[string]any)
	if !ok {
		return false
	}
	_, pk := v["publicKey"]
	_, sig := v["signature"]
	_, fp := v["fingerprint"]
	return pk || sig || fp
}

// shortCert writes a certificate as one line, such as
// "HOST-2 [GROUP-1] OVERLAY-3/24 until 2027-03-03 by FP-1". Nebula logs the
// whole certificate on handshakes; its keys and signature only take space.
func (r *reporter) shortCert(c map[string]any) string {
	m := r.m
	d, _ := c["details"].(map[string]any)
	var parts []string
	if n, ok := d["name"].(string); ok && n != "" {
		parts = append(parts, m.host(n))
	}
	var groups []string
	for _, g := range asStrings(d["groups"]) {
		groups = append(groups, m.group(g))
	}
	if len(groups) > 0 {
		parts = append(parts, "["+strings.Join(groups, ", ")+"]")
	}
	nets := asStrings(d["networks"])
	nets = append(nets, asStrings(d["ips"])...)
	for i, n := range nets {
		nets[i] = m.text(n)
	}
	if len(nets) > 0 {
		parts = append(parts, strings.Join(nets, ", "))
	}
	if na, ok := d["notAfter"].(string); ok && na != "" {
		if t, err := time.Parse(time.RFC3339, na); err == nil {
			na = t.Format(time.DateOnly)
		}
		parts = append(parts, "until "+na)
	}
	if ca, _ := d["isCa"].(bool); ca {
		parts = append(parts, "(CA)")
	}
	if is, ok := d["issuer"].(string); ok && is != "" {
		parts = append(parts, "by "+m.fingerprint(is))
	}
	return strings.Join(parts, " ")
}

func asStrings(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, e := range l {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeJSON writes v without escaping <, > and &, which only matter in
// HTML and make the log harder to read.
func writeJSON(b *bytes.Buffer, v any) {
	e := json.NewEncoder(b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		b.WriteString(`null`)
		return
	}
	b.Truncate(b.Len() - 1) // Encode adds a newline
}

// namesFromStatus are the names a status gives away, to remember for
// masking.
func namesFromStatus(st *api.StatusResponse) rememberedNames {
	var n rememberedNames
	if st == nil {
		return n
	}
	for _, ip := range st.OverlayIPs {
		if p, err := netip.ParsePrefix(ip); err == nil {
			n.Overlays = append(n.Overlays, p.Masked().String())
		}
	}
	if c := st.Cert; c != nil {
		if c.Name != "" {
			n.Hosts = append(n.Hosts, c.Name)
		}
		n.Groups = append(n.Groups, c.Groups...)
		if c.Fingerprint != "" {
			n.Fingerprint = append(n.Fingerprint, c.Fingerprint)
		}
	}
	return n
}

// namesFromSummary are the DNS names in a config summary.
func namesFromSummary(sum *api.ConfigSummary) rememberedNames {
	var n rememberedNames
	if sum == nil {
		return n
	}
	for _, lh := range sum.Lighthouses {
		if h := hostOnly(lh); !isAddr(h) {
			n.Hosts = append(n.Hosts, h)
		}
	}
	n.Hosts = append(n.Hosts, sum.StaticHostNames...)
	return n
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
