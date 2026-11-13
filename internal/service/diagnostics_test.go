// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tucono/nebula-gui/internal/api"
)

// newLoggedServer returns a server that logs at debug level to a log file
// in its state directory, as nebula-guid does.
func newLoggedServer(t *testing.T) (*Server, *LogFile) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	lf, err := OpenLogFile(dir + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lf.Close() })
	s, err := New(Options{
		Version: "1.2.3-test",
		Logger:  slog.New(slog.NewJSONHandler(lf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Engine:  &fakeEngine{},
		Store:   store,
		LogFile: lf,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, lf
}

// logRecords returns the log file's records.
func logRecords(t *testing.T, lf *LogFile) []map[string]any {
	t.Helper()
	lines, _, err := lf.Tail(api.MaxDiagnosticsLines)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		var r map[string]any
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		out = append(out, r)
	}
	return out
}

func TestRequestLogSkipsSuccessfulGets(t *testing.T) {
	srv, lf := newLoggedServer(t)
	h := srv.Handler()

	if _, ec := call(t, h, http.MethodGet, "/v1/config", nil, nil); ec != api.CodeNotConfigured {
		t.Fatalf("GET /v1/config without config = %s", ec)
	}
	putConfig(t, h, testConfig())
	for _, p := range []string{"/v1/status", "/v1/logs", "/v1/version", "/v1/diagnostics", "/v1/config"} {
		if code, ec := call(t, h, http.MethodGet, p, nil, nil); code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", p, code, ec)
		}
	}
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)

	type req struct {
		method, path string
		status       float64
	}
	var got []req
	for _, r := range logRecords(t, lf) {
		if r["msg"] == "request" {
			got = append(got, req{r["method"].(string), r["path"].(string), r["status"].(float64)})
		}
	}
	want := []req{
		{http.MethodGet, "/v1/config", http.StatusConflict},
		{http.MethodPut, "/v1/config", http.StatusOK},
		{http.MethodPost, "/v1/connect", http.StatusOK},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request lines = %+v, want %+v", got, want)
	}
}

func TestServiceStartedLine(t *testing.T) {
	srv, lf := newLoggedServer(t)
	recs := logRecords(t, lf)
	if len(recs) == 0 || recs[0]["msg"] != "service started" {
		t.Fatalf("first line = %v", recs)
	}
	r := recs[0]
	if r["service_version"] != "1.2.3-test" || r["nebula_version"] != fakeNebulaVersion ||
		r["instance_id"] != srv.instanceID || r["os"] != runtime.GOOS || r["arch"] != runtime.GOARCH ||
		r["pid"] == nil || r["api_version"] != float64(api.APIVersion) {
		t.Fatalf("service started = %v", r)
	}
	if _, ok := r["os_version"]; !ok {
		t.Fatalf("service started has no os_version: %v", r)
	}
}

func TestInstanceID(t *testing.T) {
	srv, _ := newLoggedServer(t)
	h := srv.Handler()
	putConfig(t, h, testConfig())

	var v api.VersionResponse
	var st, conn api.StatusResponse
	var lr api.LogsResponse
	var d api.DiagnosticsResponse
	call(t, h, http.MethodGet, "/v1/version", nil, &v)
	call(t, h, http.MethodGet, "/v1/status", nil, &st)
	call(t, h, http.MethodPost, "/v1/connect", nil, &conn)
	call(t, h, http.MethodGet, "/v1/logs", nil, &lr)
	call(t, h, http.MethodGet, "/v1/diagnostics", nil, &d)

	id := v.InstanceID
	if id == "" {
		t.Fatal("empty instance_id")
	}
	for name, got := range map[string]string{
		"status": st.InstanceID, "connect": conn.InstanceID, "logs": lr.InstanceID,
		"diagnostics": d.InstanceID, "diagnostics status": d.Status.InstanceID,
	} {
		if got != id {
			t.Errorf("%s instance_id = %q, want %q", name, got, id)
		}
	}

	other, _ := newLoggedServer(t)
	var v2 api.VersionResponse
	call(t, other.Handler(), http.MethodGet, "/v1/version", nil, &v2)
	if v2.InstanceID == "" || v2.InstanceID == id {
		t.Fatalf("second server instance_id = %q, first = %q", v2.InstanceID, id)
	}
}

func TestDiagnostics(t *testing.T) {
	srv, _ := newLoggedServer(t)
	h := srv.Handler()

	var d api.DiagnosticsResponse
	call(t, h, http.MethodGet, "/v1/diagnostics", nil, &d)
	if d.ServiceVersion != "1.2.3-test" || d.NebulaVersion != fakeNebulaVersion || d.APIVersion != api.APIVersion ||
		d.OS != runtime.GOOS || d.Arch != runtime.GOARCH || d.Status.State != api.StateStopped {
		t.Fatalf("diagnostics = %+v", d)
	}
	if d.ConfigSummary != nil {
		t.Fatalf("config_summary without a config = %+v", d.ConfigSummary)
	}
	if len(d.Log) == 0 || !strings.Contains(d.Log[0], `"service started"`) || d.LogTruncated {
		t.Fatalf("log = %q, truncated %v", d.Log, d.LogTruncated)
	}

	putConfig(t, h, testConfig())
	call(t, h, http.MethodPost, "/v1/connect", nil, nil)
	d = api.DiagnosticsResponse{}
	call(t, h, http.MethodGet, "/v1/diagnostics?lines=1", nil, &d)
	if d.ConfigSummary == nil || len(d.ConfigSummary.Lighthouses) != 1 || d.ConfigSummary.Lighthouses[0] != "203.0.113.1:4242" {
		t.Fatalf("config_summary = %+v", d.ConfigSummary)
	}
	if d.Status.State != api.StateRunning {
		t.Fatalf("status = %+v", d.Status)
	}
	if len(d.Log) != 1 || !d.LogTruncated || !strings.Contains(d.Log[0], `"POST"`) {
		t.Fatalf("lines=1: log = %q, truncated %v", d.Log, d.LogTruncated)
	}
	for _, l := range d.Log {
		if strings.Contains(l, "TEST KEY") {
			t.Fatal("config contents in the log")
		}
	}

	if code, ec := call(t, h, http.MethodGet, "/v1/diagnostics?lines=99999", nil, nil); code != http.StatusOK {
		t.Fatalf("lines over the maximum = %d %s, want it capped", code, ec)
	}
	for _, bad := range []string{"0", "-1", "abc"} {
		if _, ec := call(t, h, http.MethodGet, "/v1/diagnostics?lines="+bad, nil, nil); ec != api.CodeBadRequest {
			t.Errorf("lines=%s = %s, want bad_request", bad, ec)
		}
	}
}

func TestDiagnosticsWithoutLogFile(t *testing.T) {
	var d api.DiagnosticsResponse
	call(t, newTestServer(t).Handler(), http.MethodGet, "/v1/diagnostics", nil, &d)
	if d.Log == nil || len(d.Log) != 0 || d.LogTruncated {
		t.Fatalf("log = %#v", d.Log)
	}
}

// labDevConfig is the config testlab/lab.sh writes for the service under
// test, minus the PKI, which the summary ignores.
const labDevConfig = `pki:
  ca: "-"
static_host_map:
  "192.168.100.1": ["127.0.0.1:4301"]
lighthouse:
  hosts:
    - 192.168.100.1
listen:
  host: 0.0.0.0
  port: 0
firewall:
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - proto: icmp
      host: any
`

func TestSummarizeConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want *api.ConfigSummary
	}{
		{"lab", labDevConfig, &api.ConfigSummary{
			Lighthouses: []string{"127.0.0.1:4301"}, StaticHostNames: []string{},
			FirewallInbound: 1, FirewallOutbound: 1,
		}},
		{"full", `
static_host_map:
  "192.168.100.1": ["lighthouse.example.net:4242", "203.0.113.7:4242"]
  "192.168.100.2": "relay.example.net:4242"
  "192.168.100.3": ["[2001:db8::1]:4242"]
lighthouse:
  am_lighthouse: false
  hosts: ["192.168.100.1", "192.168.100.9"]
relay:
  relays: ["192.168.100.2"]
punchy:
  punch: true
listen:
  port: 4242
tun:
  routes: [{mtu: 1300, route: 10.0.0.0/16}]
  unsafe_routes: [{route: 172.30.0.0/24, via: 192.168.100.2}, {route: 172.30.1.0/24, via: 192.168.100.2}]
firewall:
  inbound: [{port: any, proto: icmp, host: any}, {port: 22, proto: tcp, group: users}, {port: 80, proto: tcp, host: any}]
  outbound: [{port: any, proto: any, host: any}]
`, &api.ConfigSummary{
			Lighthouses:     []string{"lighthouse.example.net:4242", "203.0.113.7:4242"},
			StaticHostNames: []string{"lighthouse.example.net", "relay.example.net"},
			Relays:          true, Punchy: true, ListenPort: 4242,
			TunRoutes: 1, UnsafeRoutes: 2, FirewallInbound: 3, FirewallOutbound: 1,
		}},
		{"lighthouse with old punchy", "lighthouse:\n  am_lighthouse: true\npunchy: true\n", &api.ConfigSummary{
			Lighthouses: []string{}, StaticHostNames: []string{}, AmLighthouse: true, Punchy: true,
		}},
		{"odd shapes", "lighthouse: yes\nfirewall: []\nlisten:\n  port: \"4242\"\n", &api.ConfigSummary{
			Lighthouses: []string{}, StaticHostNames: []string{},
		}},
		{"not yaml", "pki: [", nil},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeConfig(tc.yaml)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("summarizeConfig = %+v\nwant %+v", got, tc.want)
			}
		})
	}
}
