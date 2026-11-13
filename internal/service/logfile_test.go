// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLogFileRotation(t *testing.T) {
	dir := t.TempDir()
	l, err := openLogFile(dir, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	write := func(i int) {
		t.Helper()
		// 20 bytes per line, so five lines fill a file.
		if _, err := fmt.Fprintf(l, "{\"line\":%07d}  \n", i); err != nil {
			t.Fatal(err)
		}
	}
	cur, one, two := filepath.Join(dir, LogFileName), filepath.Join(dir, LogFileName+".1"), filepath.Join(dir, LogFileName+".2")

	for i := range 5 {
		write(i)
	}
	if _, err := os.Stat(one); !os.IsNotExist(err) {
		t.Fatalf("rotated before the file was full: %v", err)
	}
	write(5) // past 100 bytes: rotates first
	if got := readFile(t, cur); !strings.Contains(got, "0000005") || strings.Contains(got, "0000004") {
		t.Fatalf("current after rotation = %q", got)
	}
	if got := readFile(t, one); !strings.Contains(got, "0000000") || !strings.Contains(got, "0000004") {
		t.Fatalf(".1 after rotation = %q", got)
	}

	for i := 6; i < 15; i++ { // rotates at 10, filling .2
		write(i)
	}
	if got := readFile(t, two); !strings.Contains(got, "0000000") {
		t.Fatalf(".2 = %q", got)
	}
	write(15) // rotates at 15: the oldest file is dropped
	if got := readFile(t, two); strings.Contains(got, "0000000") || !strings.Contains(got, "0000005") {
		t.Fatalf(".2 after the third rotation = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, LogFileName+".3")); !os.IsNotExist(err) {
		t.Fatalf("kept a third old file: %v", err)
	}

	lines, truncated, err := l.Tail(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 11 || truncated || !strings.Contains(lines[0], "0000005") || !strings.Contains(lines[10], "0000015") {
		t.Fatalf("Tail = %q, %v", lines, truncated)
	}
}

func TestLogFileReopenAppends(t *testing.T) {
	dir := t.TempDir()
	for _, msg := range []string{"first\n", "second\n"} {
		l, err := OpenLogFile(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFile(t, filepath.Join(dir, LogFileName)); got != "first\nsecond\n" {
		t.Fatalf("log = %q", got)
	}
}

func TestLogFileWriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLogFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("late\n")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Write after Close = %v, want os.ErrClosed", err)
	}
	if got := readFile(t, filepath.Join(dir, LogFileName)); got != "" {
		t.Fatalf("log = %q", got)
	}
}

func TestLogFileTail(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLogFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	// .1 ends in a line cut short by a crash; the current file starts
	// after the restart.
	files := map[string]string{
		LogFileName + ".2": "a\nb\n",
		LogFileName + ".1": "c\r\n\nd-partial",
		LogFileName:        "e\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		n         int
		want      []string
		truncated bool
	}{
		{0, []string{}, false},
		{1, []string{"e"}, true},
		{3, []string{"c", "d-partial", "e"}, true},
		{5, []string{"a", "b", "c", "d-partial", "e"}, false},
		{300, []string{"a", "b", "c", "d-partial", "e"}, false},
	} {
		got, truncated, err := l.Tail(tc.n)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tc.want) || truncated != tc.truncated {
			t.Errorf("Tail(%d) = %q, %v; want %q, %v", tc.n, got, truncated, tc.want, tc.truncated)
		}
	}
}

func TestLogFileTailMissingOldFiles(t *testing.T) {
	l, err := OpenLogFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	got, truncated, err := l.Tail(10)
	if err != nil || len(got) != 0 || truncated {
		t.Fatalf("Tail of an empty log = %q, %v, %v", got, truncated, err)
	}
}

func TestLogFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses the state directory's DACL")
	}
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // loose on purpose
		t.Fatal(err)
	}
	// An existing file with loose permissions is tightened too.
	if err := os.WriteFile(filepath.Join(dir, LogFileName), nil, 0o644); err != nil { //nolint:gosec // loose on purpose
		t.Fatal(err)
	}
	l, err := OpenLogFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, LogFileName): 0o600} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := st.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}
