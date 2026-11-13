// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// appLogName is the GUI log's file name inside its directory.
	appLogName = "nebula-gui.jsonl"
	// appLogMaxBytes is the size at which the file rotates; one old file
	// (.1) is kept.
	appLogMaxBytes = 1 << 20
	// appLogKeep is how many entries stay in memory.
	appLogKeep = 500
)

// appEvent is one entry in the GUI log: something the user did, or
// something that happened to the connection.
type appEvent struct {
	Time  time.Time         `json:"time"`
	Msg   string            `json:"msg"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

// appLog is the GUI's own activity log. It records what the user did and
// what happened (service reachability, version checks, state changes,
// actions and their results, problems shown), so a troubleshooting report
// can cover the time the service wasn't reachable. It keeps the last
// appLogKeep entries in memory and appends each one to a JSON Lines file,
// so it survives a restart of the app.
//
// It never records config contents or key material, and writes paths in
// the home folder as ~/…. Callers pass file names, not full paths; see
// logPath.
type appLog struct {
	mu      sync.Mutex
	entries []appEvent // oldest first, at most appLogKeep
	file    *rotatingFile
	home    string
	now     func() time.Time
}

// newAppLog returns a log that keeps entries in memory only.
func newAppLog() *appLog {
	home, _ := os.UserHomeDir()
	return &appLog{home: home, now: time.Now}
}

// openAppLog returns a log that also writes to dir, creating it if needed,
// and loads the most recent entries already in it. On error the log still
// works, in memory only.
func openAppLog(dir string) (*appLog, error) {
	l := newAppLog()
	f, err := openRotatingFile(dir, appLogName, appLogMaxBytes)
	if err != nil {
		return l, err
	}
	l.file = f
	l.entries = f.tailEvents(appLogKeep)
	return l, nil
}

// defaultAppLogDir is <user config dir>/nebula-gui/logs.
func defaultAppLogDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "nebula-gui", "logs"), nil
}

// Add records msg with attributes given as key, value pairs. A key without
// a value is dropped.
func (l *appLog) Add(msg string, kv ...string) {
	e := appEvent{Time: l.now(), Msg: l.clean(msg)}
	for i := 0; i+1 < len(kv); i += 2 {
		if e.Attrs == nil {
			e.Attrs = map[string]string{}
		}
		e.Attrs[kv[i]] = l.clean(kv[i+1])
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	if n := len(l.entries) - appLogKeep; n > 0 {
		l.entries = append([]appEvent(nil), l.entries[n:]...)
	}
	if l.file != nil {
		b, err := json.Marshal(e)
		if err == nil {
			_, _ = l.file.Write(append(b, '\n'))
		}
	}
}

// Recent returns up to n of the newest entries, oldest first.
func (l *appLog) Recent(n int) []appEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.entries) {
		n = len(l.entries)
	}
	return append([]appEvent(nil), l.entries[len(l.entries)-n:]...)
}

// Close closes the file. Later entries stay in memory.
func (l *appLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

// clean keeps key material and the home folder's path out of the log.
// Nothing should ever pass key material in; this is a backstop.
func (l *appLog) clean(s string) string {
	if strings.Contains(s, "PRIVATE KEY") || strings.Contains(s, "BEGIN NEBULA") {
		return "[removed for safety]"
	}
	return homeToTilde(s, l.home)
}

// homeToTilde replaces the home folder's path in s with ~.
func homeToTilde(s, home string) string {
	if home == "" || home == string(filepath.Separator) {
		return s
	}
	return strings.ReplaceAll(s, home, "~")
}

// logPath is how a path appears in the GUI log: ~/… inside the home
// folder, only the file name anywhere else.
func logPath(path, home string) string {
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return filepath.Base(path)
}

// rotatingFile appends to name in dir and rotates to name.1 when the next
// write would take it past maxSize.
type rotatingFile struct {
	path    string
	maxSize int64

	mu   sync.Mutex
	f    *os.File // nil after a failed reopen; the next write retries
	size int64
}

func openRotatingFile(dir, name string, maxSize int64) (*rotatingFile, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	r := &rotatingFile{path: filepath.Join(dir, name), maxSize: maxSize}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("open log file: %w", err)
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil && r.size > 0 && r.size+int64(len(p)) > r.maxSize {
		_ = r.f.Close()
		r.f = nil
		_ = os.Rename(r.path, r.path+".1")
	}
	if r.f == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// tailEvents reads the last n entries from the old and current files,
// oldest first. Lines that don't parse are skipped.
func (r *rotatingFile) tailEvents(n int) []appEvent {
	var out []appEvent
	for _, p := range []string{r.path + ".1", r.path} {
		b, err := os.ReadFile(p) //nolint:gosec // the app's own log files
		if err != nil {          // a missing file is normal
			continue
		}
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 64<<10), appLogMaxBytes)
		for sc.Scan() {
			var e appEvent
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.Msg != "" {
				out = append(out, e)
			}
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}
