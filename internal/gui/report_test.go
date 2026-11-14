// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata")

// The fixtures use only documentation and private ranges, and made-up
// names; see testdata/report-service-log.jsonl.
var reportZone = time.FixedZone("CST", -6*60*60)

func reportTime(h, m, s, ms int) time.Time {
	return time.Date(2026, 11, 12, h, m, s, ms*int(time.Millisecond), reportZone)
}

func fixtureLog(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "report-service-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if sc.Text() != "" {
			lines = append(lines, sc.Text())
		}
	}
	return lines
}

func fixtureStatus(state api.State) api.StatusResponse {
	started := reportTime(14, 3, 20, 0)
	ca := time.Date(2036, 11, 11, 0, 0, 0, 0, time.UTC)
	st := api.StatusResponse{
		State:      state,
		OverlayIPs: []string{"192.168.100.20/24"},
		Cert: &api.CertInfo{
			Name:        "dev",
			Groups:      []string{"users", "gamers"},
			NotAfter:    time.Date(2027, 11, 11, 0, 0, 0, 0, time.UTC),
			Fingerprint: strings.Repeat("b", 64),
			CANotAfter:  &ca,
		},
		ListenPort: 4242,
		Configured: true,
		InstanceID: "i1",
	}
	if state == api.StateRunning {
		st.StartedAt = &started
	}
	return st
}

func fixtureDiag(t *testing.T, st api.StatusResponse) *api.DiagnosticsResponse {
	return &api.DiagnosticsResponse{
		ServiceVersion: "0.2.0",
		NebulaVersion:  "1.12.0",
		APIVersion:     api.APIVersion,
		InstanceID:     "i1",
		OS:             "linux",
		Arch:           "amd64",
		OSVersion:      "Ubuntu 24.04",
		Status:         st,
		ConfigSummary: &api.ConfigSummary{
			Lighthouses:      []string{"lighthouse.example.net:4242"},
			Punchy:           true,
			ListenPort:       4242,
			FirewallInbound:  3,
			FirewallOutbound: 1,
			StaticHostNames:  []string{"lighthouse.example.net"},
		},
		Log: fixtureLog(t),
	}
}

func fixtureEvents(extra ...appEvent) []appEvent {
	ev := []appEvent{
		{Time: reportTime(14, 2, 58, 1), Msg: "app started", Attrs: map[string]string{"version": "0.2.0", "os": "linux/amd64"}},
		{Time: reportTime(14, 3, 0, 120), Msg: "import done", Attrs: map[string]string{"file": "alice-laptop.yml", "restart_required": "false"}},
	}
	return append(ev, extra...)
}

func baseInput() reportInput {
	return reportInput{
		Now:        reportTime(14, 5, 0, 0),
		AppVersion: "0.2.0",
		GOOS:       "linux",
		GOARCH:     "amd64",
		Mask:       true,
		Home:       "/home/tester",
		User:       "tester",
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil { //nolint:gosec // test data
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // test data
	if err != nil {
		t.Fatalf("%v (run go test ./internal/gui -run TestReportGolden -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file; got:\n%s", name, got)
	}
}

// fixtureSecrets are the fixture's private details: none may appear in a
// masked report.
var fixtureSecrets = []string{
	"203.0.113.", "198.51.100.", "10.0.0.", "192.168.100.", "192.168.1.", "100.64.",
	"2001:db8", "fe80:", "alice", "lighthouse.example", "example.net", "gamers",
	strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("e", 64),
	strings.Repeat("c", 64), strings.Repeat("d", 64), "/home/tester", "tester",
}

func TestReportGolden(t *testing.T) {
	connectErr := &api.Error{Code: api.CodeInternal, Reason: api.ReasonTunUnavailable,
		Message: "Failed to get a tun/tap device: open /dev/net/tun: operation not permitted"}

	running := baseInput()
	st := fixtureStatus(api.StateRunning)
	running.Diag = fixtureDiag(t, st)
	running.Events = fixtureEvents(
		appEvent{Time: reportTime(14, 3, 20, 2), Msg: "connect clicked"},
		appEvent{Time: reportTime(14, 3, 20, 310), Msg: "connect done", Attrs: map[string]string{"state": "running"}},
	)

	failed := baseInput()
	fst := fixtureStatus(api.StateError)
	fst.Error, fst.ErrorReason = connectErr.Message, api.ReasonTunUnavailable
	failed.Diag = fixtureDiag(t, fst)
	failed.Problem = problemFor(connectErr, problemContext{Op: opConnect, GOOS: "linux"})
	failed.Events = fixtureEvents(
		appEvent{Time: reportTime(14, 3, 20, 2), Msg: "connect clicked"},
		appEvent{Time: reportTime(14, 3, 20, 310), Msg: "connect failed",
			Attrs: map[string]string{"problem": "tun_unavailable", "error": errorDetails(connectErr)}},
	)

	// The service can't be reached: no status, no service log. Remembered
	// names still mask the GUI log, and the remembered overlay makes its
	// addresses OVERLAY, not LAN.
	down := baseInput()
	down.Version = &api.VersionResponse{ServiceVersion: "0.2.0", NebulaVersion: "1.12.0"}
	down.DiagErr = errRefused
	down.Problem = problemFor(errRefused, problemContext{GOOS: "linux"})
	down.Names = rememberedNames{
		Hosts:    []string{"dev", "alice-laptop"},
		Groups:   []string{"users", "gamers"},
		Overlays: []string{"192.168.100.0/24"},
	}
	down.Events = fixtureEvents(
		appEvent{Time: reportTime(14, 4, 0, 0), Msg: "ping failed", Attrs: map[string]string{"error": "no answer from 192.168.100.31 (alice-laptop) via 203.0.113.10"}},
		appEvent{Time: reportTime(14, 4, 1, 0), Msg: "service unreachable", Attrs: map[string]string{"error": errRefused.Error()}},
	)

	importErr := &api.Error{Code: api.CodeInvalidConfig, Reason: api.ReasonKeyMismatch, Message: "the key in pki.key doesn't match pki.cert"}
	imp := baseInput()
	ist := fixtureStatus(api.StateStopped)
	imp.Diag = fixtureDiag(t, ist)
	imp.Diag.Log = imp.Diag.Log[:2]
	// The config's file name is the host's name, remembered from an
	// earlier connection.
	imp.Names = rememberedNames{Hosts: []string{"alice-laptop"}}
	imp.Problem = problemFor(importErr, problemContext{Op: opImport, ConfigName: "alice-laptop.yml"})
	imp.Events = fixtureEvents(
		appEvent{Time: reportTime(14, 4, 0, 0), Msg: "import started", Attrs: map[string]string{"file": "alice-laptop.yml"}},
		appEvent{Time: reportTime(14, 4, 0, 5), Msg: "import failed",
			Attrs: map[string]string{"file": "alice-laptop.yml", "problem": "key_mismatch", "error": errorDetails(importErr)}},
	)

	for name, in := range map[string]reportInput{
		"report-running":     running,
		"report-connect":     failed,
		"report-unreachable": down,
		"report-import":      imp,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := buildReport(in)
			if err != nil {
				t.Fatal(err)
			}
			checkGolden(t, name, got)
			for _, s := range fixtureSecrets {
				if strings.Contains(got, s) {
					t.Errorf("masked report contains %q", s)
				}
			}
			if len(got) > reportMaxBytes {
				t.Errorf("%d bytes", len(got))
			}
		})
	}
}

func TestReportSections(t *testing.T) {
	in := baseInput()
	in.Diag = fixtureDiag(t, fixtureStatus(api.StateRunning))
	in.Events = fixtureEvents()
	got, err := buildReport(in)
	if err != nil {
		t.Fatal(err)
	}

	// The same address has the same placeholder in every section.
	for _, s := range []string{"overlay OVERLAY-1/24", `"networks":["OVERLAY-1/24"]`, "HOST-1 [GROUP-1, GROUP-2] OVERLAY-1/24"} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q", s)
		}
	}
	// The lighthouse's name and address are the same in the config line
	// and the log.
	for _, s := range []string{"1 lighthouse (HOST-2:4242)", `"hostname":"HOST-2"`, "Resolved HOST-2 to PUBLIC-1", `"addr":"PUBLIC-1:4242"`} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q", s)
		}
	}
	// Certificates are shortened, without keys or signatures.
	if !strings.Contains(got, `"cert":"HOST-1 [GROUP-1, GROUP-2] OVERLAY-1/24 until 2027-11-11 by FP-`) {
		t.Error("certificate not shortened")
	}
	for _, s := range []string{"publicKey", "signature", "notBefore"} {
		if strings.Contains(got, s) {
			t.Errorf("contains %q", s)
		}
	}
	// Nebula's words stay.
	for _, s := range []string{"/dev/net/tun", "am_lighthouse=false", `"groups":["GROUP-1"]`, "127.0.0.1:4242", "0.0.0.0:4242", "169.254.10.20", "14:03:22", `"build":"1.12.0"`} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q", s)
		}
	}
	if strings.Contains(got, "## Problem") {
		t.Error("Problem section without a problem")
	}
	if strings.Contains(got, "journalctl") {
		t.Error("journalctl hint with the service reachable")
	}
}

func TestReportUnmasked(t *testing.T) {
	in := baseInput()
	in.Mask = false
	in.Diag = fixtureDiag(t, fixtureStatus(api.StateRunning))
	in.Events = fixtureEvents()
	got, err := buildReport(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"Masking: off", "overlay 192.168.100.20/24", "cert dev, groups users, gamers",
		"lighthouse.example.net:4242", "alice-laptop", "203.0.113.10:4242", "[2001:db8::7]:4242",
		`"cert":"dev [users, gamers] 192.168.100.20/24 until 2027-11-11 by ` + strings.Repeat("a", 64),
	} {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q", s)
		}
	}
	for _, s := range []string{"PUBLIC-", "HOST-", "OVERLAY-", "publicKey", strings.Repeat("c", 64)} {
		if strings.Contains(got, s) {
			t.Errorf("contains %q", s)
		}
	}
}

// keyMarkers are built at run time, so the source doesn't trip the secret
// check that runs before every push.
var (
	privateKeyMarker = "-----BEGIN NEBULA X25519 " + "PRIVATE KEY-----"
	nebulaCertMarker = "-----BEGIN " + "NEBULA CERTIFICATE-----"
)

func TestReportDropsKeyMaterial(t *testing.T) {
	in := baseInput()
	in.Diag = fixtureDiag(t, fixtureStatus(api.StateRunning))
	in.Diag.Log = append(in.Diag.Log,
		fmt.Sprintf(`{"time":"2026-11-12T14:04:00Z","level":"INFO","msg":"oops","key":%q}`, privateKeyMarker),
		fmt.Sprintf(`{"time":"2026-11-12T14:04:01Z","level":"INFO","msg":"%s"}`, nebulaCertMarker),
	)
	in.Events = fixtureEvents(appEvent{Time: reportTime(14, 4, 2, 0), Msg: "import failed", Attrs: map[string]string{"error": privateKeyMarker}})
	in.Problem = &problem{ID: probConfigProblem, Title: "This config has a problem", Details: "pki.key: " + privateKeyMarker}
	got, err := buildReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "4 lines removed for safety") {
		t.Errorf("no count of removed lines:\n%s", got[:300])
	}
	if unsafeText(got) {
		t.Error("key material in the report")
	}

	// A bug that lets key material through another field is caught at
	// the end.
	in.AppVersion = privateKeyMarker
	if _, err := buildReport(in); !errors.Is(err, errReportUnsafe) {
		t.Errorf("err %v", err)
	}
}

func TestReportSizeCap(t *testing.T) {
	in := baseInput()
	in.Diag = fixtureDiag(t, fixtureStatus(api.StateRunning))
	in.Diag.Log = nil
	pad := strings.Repeat("x", 200)
	for i := range api.MaxDiagnosticsLines {
		in.Diag.Log = append(in.Diag.Log, fmt.Sprintf(`{"time":"2026-11-12T14:00:00Z","level":"INFO","msg":"line %04d %s"}`, i, pad))
	}
	got, err := buildReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > reportMaxBytes {
		t.Errorf("%d bytes, over the cap", len(got))
	}
	if len(got) < reportMaxBytes-1024 {
		t.Errorf("%d bytes: the cap left too much unused", len(got))
	}
	if !strings.Contains(got, "line 1999") || strings.Contains(got, "line 0000") {
		t.Error("didn't keep the newest lines")
	}
	if !strings.Contains(got, "The oldest ") || !strings.Contains(got, "lines were left out to keep this report under 50 KB.") {
		t.Error("no note about the dropped lines")
	}
}

func TestReportFileName(t *testing.T) {
	if got := reportFileName(reportTime(23, 59, 0, 0)); got != "nebula-report-2026-11-12.md" {
		t.Errorf("got %q", got)
	}
}
