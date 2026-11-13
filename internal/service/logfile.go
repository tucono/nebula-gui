// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const (
	// LogFileName is the service log's name inside <state dir>/logs.
	LogFileName = "nebula-guid.jsonl"
	// logFileMaxBytes is the size at which the log rotates.
	logFileMaxBytes = 2 << 20
	// logFileBackups is how many rotated files are kept (.1 and .2), so
	// the log takes at most about (1+logFileBackups)*logFileMaxBytes.
	logFileBackups = 2
)

// LogFile is the persistent service log: a JSON Lines file that rotates at
// 2 MiB and keeps two old files. It is an io.Writer for slog.JSONHandler,
// which writes one whole record per call. Each record goes straight to the
// file, unbuffered, so the lines just before a crash are kept.
type LogFile struct {
	dir     string
	maxSize int64

	mu     sync.Mutex
	f      *os.File // nil after a failed reopen; the next write retries
	size   int64
	closed bool
}

// OpenLogFile creates dir if needed, restricts it like the state directory
// and opens the log in it for appending.
func OpenLogFile(dir string) (*LogFile, error) {
	return openLogFile(dir, logFileMaxBytes)
}

func openLogFile(dir string, maxSize int64) (*LogFile, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	if err := restrictDir(dir); err != nil {
		return nil, fmt.Errorf("restrict log directory: %w", err)
	}
	l := &LogFile{dir: dir, maxSize: maxSize}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

// Dir returns the directory holding the log files.
func (l *LogFile) Dir() string { return l.dir }

func (l *LogFile) path(n int) string {
	p := filepath.Join(l.dir, LogFileName)
	if n > 0 {
		p += "." + strconv.Itoa(n)
	}
	return p
}

// open opens the current file. The caller holds l.mu, or owns l.
func (l *LogFile) open() error {
	f, err := os.OpenFile(l.path(0), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	// O_CREATE's mode only applies to new files.
	if err := f.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = f.Close()
		return fmt.Errorf("open log file: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("open log file: %w", err)
	}
	l.f, l.size = f, st.Size()
	return nil
}

// Write appends p, rotating first if p would take the file past its
// maximum size. A record larger than the maximum is still written whole.
func (l *LogFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return 0, os.ErrClosed
	}
	if l.f != nil && l.size > 0 && l.size+int64(len(p)) > l.maxSize {
		l.rotate()
	}
	if l.f == nil {
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// rotate shifts the files up by one and opens a new current file. Rename
// errors are ignored: a stuck old file must not stop new lines. The caller
// holds l.mu.
func (l *LogFile) rotate() {
	_ = l.f.Close()
	l.f = nil
	_ = os.Remove(l.path(logFileBackups))
	for i := logFileBackups; i > 0; i-- {
		_ = os.Rename(l.path(i-1), l.path(i))
	}
	_ = l.open() // on failure Write retries
}

// Close closes the current file. Later writes fail; Tail still works.
func (l *LogFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// Tail returns the last n lines of the log, oldest first, read across the
// rotated files. truncated reports whether older lines were left out. A
// line cut short by a crash is returned as it is; it never runs into the
// first line of the next file.
func (l *LogFile) Tail(n int) (lines []string, truncated bool, err error) {
	if n <= 0 {
		return []string{}, false, nil
	}
	// Writes wait while the files are read, so a rotation can't move them.
	// Reading at most 6 MiB takes milliseconds.
	l.mu.Lock()
	defer l.mu.Unlock()

	var rev []string // newest first
	for i := 0; i <= logFileBackups; i++ {
		b, err := os.ReadFile(l.path(i))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, false, fmt.Errorf("read log file: %w", err)
		}
		fileLines := splitLines(b)
		for j := len(fileLines) - 1; j >= 0; j-- {
			if len(rev) == n {
				return reverse(rev), true, nil
			}
			rev = append(rev, fileLines[j])
		}
	}
	return reverse(rev), false, nil
}

// splitLines splits b into lines, dropping empty ones. A last line without
// a newline is kept.
func splitLines(b []byte) []string {
	var out []string
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		var line []byte
		if i < 0 {
			line, b = b, nil
		} else {
			line, b = b[:i], b[i+1:]
		}
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) > 0 {
			out = append(out, string(line))
		}
	}
	return out
}

func reverse(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}
