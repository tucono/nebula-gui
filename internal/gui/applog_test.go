// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAppLogMemory(t *testing.T) {
	l := newAppLog()
	l.Add("connect clicked")
	l.Add("connect failed", "problem", "tun_unavailable", "dangling")
	got := l.Recent(10)
	if len(got) != 2 || got[0].Msg != "connect clicked" || got[1].Attrs["problem"] != "tun_unavailable" {
		t.Fatalf("Recent = %+v", got)
	}
	if _, ok := got[1].Attrs["dangling"]; ok {
		t.Fatal("kept a key without a value")
	}
	if r := l.Recent(1); len(r) != 1 || r[0].Msg != "connect failed" {
		t.Fatalf("Recent(1) = %+v", r)
	}

	for i := range appLogKeep + 10 {
		l.Add(fmt.Sprint(i))
	}
	got = l.Recent(appLogKeep + 100)
	if len(got) != appLogKeep || got[0].Msg != "10" || got[len(got)-1].Msg != fmt.Sprint(appLogKeep+9) {
		t.Fatalf("kept %d entries, %q … %q", len(got), got[0].Msg, got[len(got)-1].Msg)
	}
}

func TestAppLogClean(t *testing.T) {
	l := newAppLog()
	l.home = filepath.FromSlash("/home/alice")
	l.Add("import", "file", filepath.FromSlash("/home/alice/nebula/config.yml"))
	l.Add("oops -----BEGIN NEBULA ED25519 PRIVATE KEY-----")
	l.Add("x", "pem", "-----BEGIN NEBULA CERTIFICATE-----")
	got := l.Recent(3)
	if f := got[0].Attrs["file"]; f != filepath.FromSlash("~/nebula/config.yml") {
		t.Errorf("home path logged as %q", f)
	}
	for _, e := range got[1:] {
		if strings.Contains(e.Msg+e.Attrs["pem"], "BEGIN") {
			t.Errorf("key material kept: %+v", e)
		}
	}
}

func TestLogPath(t *testing.T) {
	home := filepath.FromSlash("/home/alice")
	tests := []struct{ path, want string }{
		{"/home/alice/Downloads/config.yml", "~/Downloads/config.yml"},
		{"/home/alice", "~"},
		{"/home/alicex/config.yml", "config.yml"},
		{"/etc/nebula/host.key", "host.key"},
	}
	for _, tt := range tests {
		if got := logPath(filepath.FromSlash(tt.path), home); got != tt.want {
			t.Errorf("logPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
	if got := logPath(filepath.FromSlash("/etc/nebula/host.key"), ""); got != "host.key" {
		t.Errorf("without a home folder: %q", got)
	}
}

func TestAppLogFile(t *testing.T) {
	dir := t.TempDir()
	l, err := openAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	l.now = func() time.Time { return time.Date(2026, 11, 13, 14, 3, 20, 0, time.UTC) }
	l.Add("connect clicked", "state", "stopped")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l.Add("after close") // memory only

	p := filepath.Join(dir, appLogName)
	b, err := os.ReadFile(p) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	want := `{"time":"2026-11-13T14:03:20Z","msg":"connect clicked","attrs":{"state":"stopped"}}` + "\n"
	if string(b) != want {
		t.Fatalf("file = %q, want %q", b, want)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("file mode %v", st.Mode().Perm())
		}
	}

	// A new log, as after an app restart, starts with the old entries.
	l2, err := openAppLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l2.Close() }()
	if got := l2.Recent(10); len(got) != 1 || got[0].Msg != "connect clicked" {
		t.Fatalf("reloaded %+v", got)
	}
}

func TestRotatingFile(t *testing.T) {
	dir := t.TempDir()
	r, err := openRotatingFile(dir, "x.jsonl", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	line := func(i int) []byte {
		return []byte(fmt.Sprintf(`{"time":"2026-11-13T00:00:00Z","msg":"%02d"}`+"\n", i)) // 45 bytes
	}
	for i := range 5 {
		if _, err := r.Write(line(i)); err != nil {
			t.Fatal(err)
		}
	}
	// 0 and 1 fill the first file, 2 and 3 the second, and 4 rotates
	// again, dropping 0 and 1.
	old, _ := os.ReadFile(filepath.Join(dir, "x.jsonl.1")) //nolint:gosec // a test file
	cur, _ := os.ReadFile(filepath.Join(dir, "x.jsonl"))   //nolint:gosec // a test file
	if string(old) != string(line(2))+string(line(3)) || string(cur) != string(line(4)) {
		t.Fatalf("old %q, current %q", old, cur)
	}
	got := r.tailEvents(10)
	if len(got) != 3 || got[0].Msg != "02" || got[2].Msg != "04" {
		t.Fatalf("tailEvents = %+v", got)
	}
	if got := r.tailEvents(1); len(got) != 1 || got[0].Msg != "04" {
		t.Fatalf("tailEvents(1) = %+v", got)
	}
}
