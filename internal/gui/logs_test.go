// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/tucono/nebula-gui/internal/api"
)

func (f *fakeService) Logs(_ context.Context, since uint64) (*api.LogsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("logs %d", since))
	if f.down {
		return nil, errRefused
	}
	r := &api.LogsResponse{Next: uint64(len(f.logLines)), InstanceID: f.instance, Lines: []api.LogLine{}}
	for _, l := range f.logLines {
		if l.Seq >= since {
			r.Lines = append(r.Lines, l)
		}
	}
	return r, nil
}

// addLogs appends lines with the messages given, numbered on from the
// last.
func (f *fakeService) addLogs(level string, msgs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range msgs {
		f.logLines = append(f.logLines, api.LogLine{
			Seq: uint64(len(f.logLines)), Time: time.Date(2026, 11, 14, 14, 3, 22, 0, time.Local),
			Level: level, Msg: m,
		})
	}
}

func lineMsgs(ls []api.LogLine) []string {
	out := []string{}
	for _, l := range ls {
		out = append(out, l.Msg)
	}
	return out
}

func TestFormatLogLine(t *testing.T) {
	at := time.Date(2026, 11, 14, 14, 3, 22, 500, time.Local)
	tests := []struct {
		name string
		line api.LogLine
		want string
	}{
		{"plain", api.LogLine{Time: at, Level: "info", Msg: "service started"},
			"14:03:22 INFO  service started"},
		{"nebula with fields sorted", api.LogLine{Time: at, Level: "info", Msg: "Handshake message sent",
			Fields: map[string]any{"component": "nebula", "vpnAddrs": "[192.168.100.1]", "initiatorIndex": float64(4242), "cached": false}},
			"14:03:22 INFO  [nebula] Handshake message sent cached=false initiatorIndex=4242 vpnAddrs=[192.168.100.1]"},
		{"quoted values", api.LogLine{Time: at, Level: "error", Msg: "connect failed",
			Fields: map[string]any{"error": "no such file", "empty": "", "eq": "a=b", "rtt": 1.5}},
			`14:03:22 ERROR connect failed empty="" eq="a=b" error="no such file" rtt=1.5`},
		{"warn and nested value", api.LogLine{Time: at, Level: "warn", Msg: "odd",
			Fields: map[string]any{"list": []any{"a", float64(1)}, "nil": nil}},
			`14:03:22 WARN  odd list="[\"a\",1]" nil=""`},
		{"utc time shown local", api.LogLine{Time: at.UTC(), Level: "debug", Msg: "x"},
			"14:03:22 DEBUG x"},
	}
	for _, tt := range tests {
		if got := formatLogLine(tt.line); got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, got, tt.want)
		}
	}
}

func TestLogPollerFetch(t *testing.T) {
	svc := newFakeService()
	p := newLogPoller(svc)
	ctx := context.Background()

	svc.addLogs("info", "a", "b")
	u := p.fetch(ctx)
	if u.Err != nil || !u.Reset || !equal(lineMsgs(u.Lines), []string{"a", "b"}) {
		t.Fatalf("first read: %+v", u)
	}
	svc.addLogs("info", "c")
	u = p.fetch(ctx)
	if u.Reset || !equal(lineMsgs(u.Lines), []string{"c"}) {
		t.Fatalf("second read: %+v", u)
	}
	if u = p.fetch(ctx); u.Reset || len(u.Lines) != 0 {
		t.Fatalf("nothing new: %+v", u)
	}
	if c := svc.takeCalls(); !equal(c, []string{"logs 0", "logs 2", "logs 3"}) {
		t.Fatalf("calls %v", c)
	}

	// Unreachable: an error, and the next read carries on.
	svc.set(func(f *fakeService) { f.down = true })
	if u = p.fetch(ctx); u.Err == nil {
		t.Fatal("no error while down")
	}
	svc.set(func(f *fakeService) { f.down = false })
	svc.addLogs("info", "d")
	if u = p.fetch(ctx); u.Reset || !equal(lineMsgs(u.Lines), []string{"d"}) {
		t.Fatalf("after recovery: %+v", u)
	}

	// Restart, and the new service has logged more lines than were seen:
	// the read from the old position would skip its first lines.
	svc.set(func(f *fakeService) { f.instance, f.logLines = "i2", nil })
	svc.addLogs("info", "n0", "n1", "n2", "n3", "n4", "n5")
	svc.takeCalls()
	u = p.fetch(ctx)
	if !u.Reset || !equal(lineMsgs(u.Lines), []string{"n0", "n1", "n2", "n3", "n4", "n5"}) {
		t.Fatalf("after restart: %+v", u)
	}
	if c := svc.takeCalls(); !equal(c, []string{"logs 4", "logs 0"}) {
		t.Fatalf("restart calls %v", c)
	}
	if u = p.fetch(ctx); u.Reset || len(u.Lines) != 0 {
		t.Fatalf("after restart, nothing new: %+v", u)
	}
}

func TestLogModel(t *testing.T) {
	m := newLogModel()
	m.max = 3
	if m.State() != "Reading the log…" {
		t.Fatalf("state %q", m.State())
	}
	if m.apply(logUpdate{Reset: true}) || m.State() != "Nothing logged yet." {
		t.Fatalf("empty: %q", m.State())
	}
	line := func(level, msg string) api.LogLine { return api.LogLine{Level: level, Msg: msg} }
	if m.apply(logUpdate{Lines: []api.LogLine{line("info", "a"), line("warn", "b")}}) {
		t.Fatal("appending shifted the rows")
	}
	if m.Len() != 2 || m.State() != "" || m.Text(5) != "" {
		t.Fatalf("len %d, state %q", m.Len(), m.State())
	}
	if !m.apply(logUpdate{Lines: []api.LogLine{line("info", "c"), line("error", "d")}}) {
		t.Fatal("trimming didn't report a shift")
	}
	var got []string
	for i := range m.Len() {
		got = append(got, m.entries[m.view[i]].line.Msg)
	}
	if !equal(got, []string{"b", "c", "d"}) {
		t.Fatalf("kept %v", got)
	}

	m.setWarnOnly(true)
	if m.Len() != 2 || m.entries[m.view[0]].line.Msg != "b" {
		t.Fatalf("warn only: %d rows", m.Len())
	}

	m.apply(logUpdate{Err: errRefused})
	if m.Len() != 2 || m.State() != "Can't reach the background service. These are the last lines it sent." {
		t.Fatalf("error kept the lines? %d, %q", m.Len(), m.State())
	}
	if !m.apply(logUpdate{Reset: true, Lines: []api.LogLine{line("info", "new")}}) {
		t.Fatal("reset didn't report a shift")
	}
	if m.Len() != 0 || m.State() != "No warnings or errors." {
		t.Fatalf("after reset: %d, %q", m.Len(), m.State())
	}
	m.setWarnOnly(false)
	if m.Len() != 1 || m.Text(0) == "" {
		t.Fatalf("all: %d", m.Len())
	}

	fresh := newLogModel()
	fresh.apply(logUpdate{Err: errRefused})
	if fresh.State() != "Can't reach the background service, so its log can't be read." {
		t.Fatalf("fresh error: %q", fresh.State())
	}
}

func TestMainWindowLogs(t *testing.T) {
	svc := newFakeService()
	m, _ := testWindow(t, svc)
	readLogs := func() { m.logs.apply(m.logPoller.fetch(context.Background())) }

	svc.addLogs("info", "service started")
	svc.addLogs("warn", "handshake timed out")
	readLogs()
	if m.logs.list.Length() != 2 || m.logs.state.Text != "" {
		t.Fatalf("rows %d, state %q", m.logs.list.Length(), m.logs.state.Text)
	}

	if m.logPoller.fast.Load() {
		t.Fatal("fast while the Status tab shows")
	}
	m.tabs.SelectIndex(1)
	if !m.logsShowing() || !m.logPoller.fast.Load() {
		t.Fatal("not fast with the Logs tab showing")
	}

	m.logs.list.Select(1)
	if m.logs.detail.Text != "14:03:22 WARN  handshake timed out" {
		t.Fatalf("detail %q", m.logs.detail.Text)
	}
	test.Tap(m.logs.warnOnly)
	if m.logs.list.Length() != 1 || m.logs.detail.Text != logDetailHint {
		t.Fatalf("warn only: rows %d, detail %q", m.logs.list.Length(), m.logs.detail.Text)
	}

	m.tabs.SelectIndex(0)
	if m.logsShowing() || m.logPoller.fast.Load() {
		t.Fatal("still fast on the Status tab")
	}
}
