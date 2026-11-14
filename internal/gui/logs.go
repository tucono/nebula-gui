// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	// maxLogLines is how many service log lines the GUI keeps, the same as
	// the service's ring buffer.
	maxLogLines = 2000
	// logFastInterval is how often the log is read while the Logs tab is
	// showing; logSlowInterval the rest of the time, and while the service
	// can't be reached.
	logFastInterval = 2 * time.Second
	logSlowInterval = 10 * time.Second
)

// logsClient is the part of api.Client the log poller uses.
type logsClient interface {
	Logs(ctx context.Context, since uint64) (*api.LogsResponse, error)
}

// logUpdate is what one read of the service log learned.
type logUpdate struct {
	// Reset is set when the service restarted: the lines kept so far are
	// from the previous instance and Lines starts from its beginning.
	Reset bool
	Lines []api.LogLine
	Err   error
}

// logPoller reads new lines from GET /v1/logs: every logFastInterval while
// fast is set, otherwise every logSlowInterval.
type logPoller struct {
	c    logsClient
	fast atomic.Bool
	wake chan struct{}

	// Only the polling goroutine touches these.
	since    uint64
	instance string
}

func newLogPoller(c logsClient) *logPoller {
	return &logPoller{c: c, wake: make(chan struct{}, 1)}
}

// SetFast switches between the fast and slow interval. Switching to fast
// reads at once.
func (p *logPoller) SetFast(on bool) {
	if p.fast.Swap(on) != on && on {
		p.Poke()
	}
}

// Poke makes the poller read now instead of waiting.
func (p *logPoller) Poke() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// run reads until ctx ends, passing each update to deliver on the polling
// goroutine.
func (p *logPoller) run(ctx context.Context, deliver func(logUpdate)) {
	for {
		u := p.fetch(ctx)
		if ctx.Err() != nil {
			return
		}
		deliver(u)
		delay := logSlowInterval
		if p.fast.Load() && u.Err == nil {
			delay = logFastInterval
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		case <-p.wake:
			t.Stop()
		}
	}
}

// fetch reads the lines added since the last read. When the instance ID
// changes, the service has restarted and its sequence numbers started
// again at 0, so it reads again from 0. Checking Next < since instead
// would miss a restart once the new service has logged more lines than
// the GUI had seen.
func (p *logPoller) fetch(ctx context.Context) logUpdate {
	resp, err := p.logs(ctx, p.since)
	if err != nil {
		return logUpdate{Err: err}
	}
	var u logUpdate
	if resp.InstanceID != p.instance {
		u.Reset = true
		if p.since != 0 {
			if resp, err = p.logs(ctx, 0); err != nil {
				// Read from 0 next time, and reset then.
				p.since, p.instance = 0, ""
				return logUpdate{Err: err}
			}
		}
		p.instance = resp.InstanceID
	}
	p.since = resp.Next
	u.Lines = resp.Lines
	return u
}

func (p *logPoller) logs(ctx context.Context, since uint64) (*api.LogsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	return p.c.Logs(ctx, since)
}

// logEntry is a kept line with its display text.
type logEntry struct {
	line api.LogLine
	text string
}

// logModel holds the last maxLogLines lines and the ones the Logs tab
// shows. It is only touched on the UI goroutine.
type logModel struct {
	max      int
	entries  []logEntry
	warnOnly bool
	// view holds the indexes into entries that are shown.
	view []int
	// err is the last read's error, or nil once a read succeeds.
	err error
	// read is set once any read has succeeded.
	read bool
}

func newLogModel() *logModel {
	return &logModel{max: maxLogLines}
}

// apply adds an update. It reports whether rows already shown moved or
// went away, so a selection by index is no longer valid.
func (m *logModel) apply(u logUpdate) (shifted bool) {
	if u.Err != nil {
		m.err = u.Err
		return false
	}
	m.err, m.read = nil, true
	if u.Reset && len(m.entries) > 0 {
		m.entries = m.entries[:0]
		shifted = true
	}
	for _, l := range u.Lines {
		m.entries = append(m.entries, logEntry{line: l, text: formatLogLine(l)})
	}
	if over := len(m.entries) - m.max; over > 0 {
		m.entries = append(m.entries[:0:0], m.entries[over:]...)
		shifted = true
	}
	m.rebuild()
	return shifted
}

// setWarnOnly shows only warnings and errors, or every line.
func (m *logModel) setWarnOnly(on bool) {
	m.warnOnly = on
	m.rebuild()
}

func (m *logModel) rebuild() {
	m.view = m.view[:0]
	for i, e := range m.entries {
		if !m.warnOnly || isWarnOrWorse(e.line.Level) {
			m.view = append(m.view, i)
		}
	}
}

// Len is the number of rows shown.
func (m *logModel) Len() int { return len(m.view) }

// Text is the display text of row i, or "" when out of range.
func (m *logModel) Text(i int) string {
	if i < 0 || i >= len(m.view) {
		return ""
	}
	return m.entries[m.view[i]].text
}

// State is the line above the rows: why there are none, or that the log
// can't be read just now. It's empty when there's nothing to say.
func (m *logModel) State() string {
	switch {
	case m.err != nil && len(m.entries) > 0:
		return "Can't reach the background service. These are the last lines it sent."
	case m.err != nil:
		return "Can't reach the background service, so its log can't be read."
	case !m.read:
		return "Reading the log…"
	case len(m.entries) == 0:
		return "Nothing logged yet."
	case len(m.view) == 0:
		return "No warnings or errors."
	default:
		return ""
	}
}

func isWarnOrWorse(level string) bool {
	l := strings.ToLower(level)
	return strings.HasPrefix(l, "warn") || strings.HasPrefix(l, "error") ||
		l == "fatal" || l == "panic"
}

// formatLogLine is the display form of a service log line:
//
//	15:04:05 INFO  [nebula] msg key=val …
//
// The time is local, fields are sorted by key, and the component field
// becomes the prefix. It's for showing only: the troubleshooting report
// uses the service's full JSON lines.
func formatLogLine(l api.LogLine) string {
	var b strings.Builder
	b.WriteString(l.Time.Local().Format("15:04:05"))
	b.WriteByte(' ')
	fmt.Fprintf(&b, "%-5s", strings.ToUpper(l.Level))
	b.WriteByte(' ')

	keys := make([]string, 0, len(l.Fields))
	for k := range l.Fields {
		if k != "component" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if c, ok := l.Fields["component"]; ok {
		if s := fieldValue(c); s != "" {
			b.WriteString("[" + s + "] ")
		}
	}
	b.WriteString(l.Msg)
	for _, k := range keys {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(quoteIfNeeded(fieldValue(l.Fields[k])))
	}
	return b.String()
}

// fieldValue writes a field value from the JSON response as text. Numbers
// arrive as float64; whole ones are written without a decimal point.
func fieldValue(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		if j, err := json.Marshal(v); err == nil {
			return string(j)
		}
		return fmt.Sprint(v)
	}
}

// quoteIfNeeded quotes values that would otherwise read as several fields.
func quoteIfNeeded(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\n\r\"=") {
		return strconv.Quote(s)
	}
	return s
}
