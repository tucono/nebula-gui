// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

func TestLogBufferSince(t *testing.T) {
	b := NewLogBuffer(3)
	l := slog.New(b.Handler(slog.LevelInfo))

	if r := b.Since(0); r.Next != 0 || len(r.Lines) != 0 || r.Lines == nil {
		t.Fatalf("empty buffer: %+v", r)
	}

	for i := range 5 {
		l.Info(fmt.Sprint("m", i))
	}
	l.Debug("dropped: below level")

	r := b.Since(0)
	if r.Next != 5 || len(r.Lines) != 3 {
		t.Fatalf("Since(0) = %+v", r)
	}
	for i, line := range r.Lines {
		if want := uint64(i + 2); line.Seq != want || line.Msg != fmt.Sprint("m", want) {
			t.Errorf("line %d = %+v, want seq %d", i, line, want)
		}
	}
	if r := b.Since(4); len(r.Lines) != 1 || r.Lines[0].Seq != 4 {
		t.Fatalf("Since(4) = %+v", r)
	}
	if r := b.Since(5); len(r.Lines) != 0 || r.Next != 5 {
		t.Fatalf("Since(5) = %+v", r)
	}
	if r := b.Since(99); len(r.Lines) != 0 || r.Next != 5 {
		t.Fatalf("Since(99) = %+v", r)
	}
}

func TestLogBufferFields(t *testing.T) {
	b := NewLogBuffer(10)
	l := slog.New(b.Handler(slog.LevelInfo)).With("component", "nebula").WithGroup("g")
	l.Warn("Handshake message sent",
		"err", errors.New("boom"),
		"n", 3,
		"ok", true,
		"d", 2*time.Second,
		slog.Group("sub", "x", "y"),
	)
	line := b.Since(0).Lines[0]
	if line.Level != "warn" || line.Msg != "Handshake message sent" || line.Time.IsZero() {
		t.Fatalf("line = %+v", line)
	}
	want := map[string]any{
		"component": "nebula",
		"g.err":     "boom",
		"g.n":       int64(3),
		"g.ok":      true,
		"g.d":       "2s",
		"g.sub.x":   "y",
	}
	for k, v := range want {
		if line.Fields[k] != v {
			t.Errorf("fields[%q] = %#v, want %#v", k, line.Fields[k], v)
		}
	}
	if len(line.Fields) != len(want) {
		t.Errorf("fields = %v", line.Fields)
	}
}
