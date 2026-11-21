// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// DefaultLogLines is how many log lines the ring buffer keeps.
const DefaultLogLines = 2000

// LogBuffer is a ring buffer of recent log lines, served by GET /v1/logs.
// Feed it through the slog.Handler returned by Handler.
type LogBuffer struct {
	mu    sync.Mutex
	lines []api.LogLine // ring storage, len == capacity once full
	start int           // index of the oldest line once full
	next  uint64        // seq of the next line
}

// NewLogBuffer returns a buffer that keeps the last n lines.
func NewLogBuffer(n int) *LogBuffer {
	if n <= 0 {
		n = DefaultLogLines
	}
	return &LogBuffer{lines: make([]api.LogLine, 0, n)}
}

func (b *LogBuffer) add(l api.LogLine) {
	b.mu.Lock()
	defer b.mu.Unlock()
	l.Seq = b.next
	b.next++
	if len(b.lines) < cap(b.lines) {
		b.lines = append(b.lines, l)
		return
	}
	b.lines[b.start] = l
	b.start = (b.start + 1) % len(b.lines)
}

// Since returns the buffered lines with seq >= since, oldest first, and the
// seq to ask for next time.
func (b *LogBuffer) Since(since uint64) api.LogsResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := api.LogsResponse{Next: b.next, Lines: []api.LogLine{}}
	n := len(b.lines)
	if n == 0 || since >= b.next {
		return out
	}
	oldest := b.next - uint64(n)
	skip := 0
	if since > oldest {
		skip = int(since - oldest) //nolint:gosec // since < b.next, so this is < n
	}
	out.Lines = make([]api.LogLine, 0, n-skip)
	for i := skip; i < n; i++ {
		out.Lines = append(out.Lines, b.lines[(b.start+i)%n])
	}
	return out
}

// Handler returns a slog.Handler that records into b at level and above.
func (b *LogBuffer) Handler(level slog.Leveler) slog.Handler {
	return &bufHandler{buf: b, level: level}
}

type bufHandler struct {
	buf    *LogBuffer
	level  slog.Leveler
	attrs  []slog.Attr // already prefixed with their groups
	prefix string      // open group prefix, for example "a.b."
}

func (h *bufHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *bufHandler) Handle(_ context.Context, r slog.Record) error {
	var fields map[string]any
	if len(h.attrs) > 0 || r.NumAttrs() > 0 {
		fields = make(map[string]any, len(h.attrs)+r.NumAttrs())
		for _, a := range h.attrs {
			addField(fields, "", a)
		}
		r.Attrs(func(a slog.Attr) bool {
			addField(fields, h.prefix, a)
			return true
		})
	}
	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	h.buf.add(api.LogLine{
		Time:   t,
		Level:  strings.ToLower(r.Level.String()),
		Msg:    r.Message,
		Fields: fields,
	})
	return nil
}

func (h *bufHandler) WithAttrs(as []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = make([]slog.Attr, 0, len(h.attrs)+len(as))
	nh.attrs = append(nh.attrs, h.attrs...)
	for _, a := range as {
		a.Key = h.prefix + a.Key
		nh.attrs = append(nh.attrs, a)
	}
	return &nh
}

func (h *bufHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	nh.prefix = h.prefix + name + "."
	return &nh
}

// addField flattens a into fields, joining group keys with dots, and turns
// values into JSON-friendly types.
func addField(fields map[string]any, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if a.Key == "" && v.Kind() != slog.KindGroup {
		return
	}
	switch v.Kind() {
	case slog.KindGroup:
		p := prefix
		if a.Key != "" {
			p += a.Key + "."
		}
		for _, ga := range v.Group() {
			addField(fields, p, ga)
		}
		return
	case slog.KindString:
		fields[prefix+a.Key] = v.String()
	case slog.KindInt64:
		fields[prefix+a.Key] = v.Int64()
	case slog.KindUint64:
		fields[prefix+a.Key] = v.Uint64()
	case slog.KindFloat64:
		fields[prefix+a.Key] = v.Float64()
	case slog.KindBool:
		fields[prefix+a.Key] = v.Bool()
	case slog.KindDuration:
		fields[prefix+a.Key] = v.Duration().String()
	case slog.KindTime:
		fields[prefix+a.Key] = v.Time().Format(time.RFC3339Nano)
	default:
		fields[prefix+a.Key] = anyString(v.Any())
	}
}

func anyString(x any) string {
	switch x := x.(type) {
	case nil:
		return ""
	case error:
		return x.Error()
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}
