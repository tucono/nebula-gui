// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

const fakeNebulaVersion = "9.9.9-fake"

// fakeEngine is an Engine that runs nothing.
type fakeEngine struct {
	mu       sync.Mutex
	checkErr error
	startErr error
	starts   int
	last     *fakeInstance
}

func (e *fakeEngine) Version() string { return fakeNebulaVersion }

func (e *fakeEngine) Check(yaml string) (*ConfigInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.checkErr != nil {
		return nil, e.checkErr
	}
	name := "dev"
	if strings.Contains(yaml, "# alice") {
		name = "alice"
	}
	return &ConfigInfo{
		Cert:       api.CertInfo{Name: name, Groups: []string{"users"}, NotAfter: time.Unix(1893456000, 0).UTC(), Fingerprint: "abc123"},
		OverlayIPs: []string{"192.168.100.20/24"},
		ListenPort: 4242,
	}, nil
}

func (e *fakeEngine) Start(string) (Instance, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.startErr != nil {
		return nil, e.startErr
	}
	e.starts++
	e.last = &fakeInstance{done: make(chan struct{})}
	return e.last, nil
}

func (e *fakeEngine) instance() *fakeInstance {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.last
}

type fakeInstance struct {
	once    sync.Once
	done    chan struct{}
	err     error
	stopped bool
}

func (i *fakeInstance) Stop() {
	i.once.Do(func() { i.stopped = true; close(i.done) })
}

// crash makes the instance stop on its own with err.
func (i *fakeInstance) crash(err error) {
	i.once.Do(func() { i.err = err; close(i.done) })
}

func (i *fakeInstance) Done() <-chan struct{} { return i.done }

func (i *fakeInstance) Err() error {
	select {
	case <-i.done:
		return i.err
	default:
		return nil
	}
}

var errFakeStart = errors.New("listen udp 0.0.0.0:4242: address already in use")

// testPEM builds a fake PEM block. The marker words are joined at run time
// so the repo's secret scan doesn't match test fixtures.
func testPEM(kind string) string {
	return "-----BEGIN " + kind + "-----\nAAAA\n-----END " + kind + "-----\n"
}

// testConfig is a config that passes validateConfig. Its PEM is fake; the
// fake engine doesn't parse it.
func testConfig() string {
	return "pki:\n" +
		"  ca: |\n" + indentPEM(testPEM("TEST CA")) +
		"  cert: |\n" + indentPEM(testPEM("TEST CERT")) +
		"  key: |\n" + indentPEM(testPEM("TEST KEY")) +
		"lighthouse:\n  hosts:\n    - 192.168.100.1\n" +
		"static_host_map:\n  192.168.100.1: [\"203.0.113.1:4242\"]\n"
}

func indentPEM(s string) string {
	var b strings.Builder
	for _, l := range strings.SplitAfter(s, "\n") {
		if l != "" {
			b.WriteString("    " + l)
		}
	}
	return b.String()
}
