// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestServiceNotRunningSetup(t *testing.T) {
	installed := serviceSetup{Known: true, Installed: true}
	startable := serviceSetup{Known: true, Installed: true, CanStart: true}
	for _, tc := range []struct {
		name  string
		goos  string
		setup serviceSetup
		id    problemID
		fix   fixKind
		step  string // in the first step
		cmd   string // in the command; "" for none
	}{
		{"unknown", "linux", serviceSetup{}, probServiceNotRunning, fixRetry, "Restart", ""},
		{"not installed", "linux", serviceSetup{Known: true}, probServiceNotInstalled, fixOpenDownloads, "Download", ""},
		{"not installed, windows", "windows", serviceSetup{Known: true}, probServiceNotInstalled, fixOpenDownloads, "Download", ""},
		{"linux", "linux", startable, probServiceNotRunning, fixStartService, "password", ""},
		{"linux without systemd", "linux", installed, probServiceNotRunning, fixRetry, "Restart", ""},
		{"windows", "windows", startable, probServiceNotRunning, fixStartService, "Windows asks", ""},
		{"macos", "darwin", installed, probServiceNotRunning, fixRetry, "Terminal", "launchctl kickstart system/io.github.tucono.nebula-guid"},
	} {
		p := serviceNotRunning(tc.goos, tc.setup)
		if p.ID != tc.id || p.Fix == nil || p.Fix.Kind != tc.fix {
			t.Errorf("%s: %s, fix %+v", tc.name, p.ID, p.Fix)
			continue
		}
		if !strings.Contains(p.Steps[0], tc.step) {
			t.Errorf("%s: first step %q doesn't mention %q", tc.name, p.Steps[0], tc.step)
		}
		if (tc.cmd == "") != (p.Command == "") || !strings.Contains(p.Command, tc.cmd) {
			t.Errorf("%s: command %q, want %q", tc.name, p.Command, tc.cmd)
		}
	}
}

func TestStartFailed(t *testing.T) {
	if p := startFailed("linux", "Interactive authentication required."); p.Command != "sudo systemctl start nebula-guid" ||
		p.Details != "Interactive authentication required." {
		t.Errorf("linux: %+v", p)
	}
	if p := startFailed("windows", "x"); p.Command != "" || !strings.Contains(p.Steps[0], "Restart") {
		t.Errorf("windows: %+v", p)
	}
}

func TestProblemForSetup(t *testing.T) {
	refused := wrapLikeClient(notRunningErrnos[0])
	denied := wrapLikeClient(permissionErrnos[0])
	for _, tc := range []struct {
		name  string
		err   error
		setup serviceSetup
		want  problemID
	}{
		{"refused, unknown", refused, serviceSetup{}, probServiceNotRunning},
		{"refused, not installed", refused, serviceSetup{Known: true}, probServiceNotInstalled},
		{"refused, installed", refused, serviceSetup{Known: true, Installed: true}, probServiceNotRunning},
		{"denied", denied, serviceSetup{Known: true, Installed: true}, probNoPermission},
		{"denied, group pending", denied, serviceSetup{Known: true, Installed: true, GroupPending: true}, probRestartNeeded},
		// A pending group explains being refused, not a stopped service.
		{"refused, group pending", refused, serviceSetup{Known: true, Installed: true, GroupPending: true}, probServiceNotRunning},
		{"timeout", timeoutErr{}, serviceSetup{Known: true}, probServiceNotResponding},
	} {
		if p := problemFor(tc.err, problemContext{GOOS: "linux", Setup: tc.setup}); p.ID != tc.want {
			t.Errorf("%s: %s, want %s", tc.name, p.ID, tc.want)
		}
	}
}

func TestGroupPendingIn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []string
		session []int
		want    bool
	}{
		{"not a member", []string{"1000", "27"}, []int{1000, 27}, false},
		{"member since sign-in", []string{"1000", "991"}, []int{1000}, true},
		{"member in this session", []string{"1000", "991"}, []int{1000, 991}, false},
		{"primary group", []string{"991"}, []int{991}, false},
	} {
		if got := groupPendingIn("991", tc.members, tc.session); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPollerProbesWhenDown(t *testing.T) {
	svc := newFakeService()
	log := newAppLog()
	p := newPoller(svc, log, &statusGate{})
	probes := 0
	p.probe = func() serviceSetup {
		probes++
		return serviceSetup{Known: true}
	}
	if r, _ := p.poll(context.Background()); r.Err != nil || probes != 0 {
		t.Fatalf("probed while up: %d, %v", probes, r.Err)
	}
	svc.set(func(f *fakeService) { f.down = true })
	r, _ := p.poll(context.Background())
	if probes != 1 || r.Setup != (serviceSetup{Known: true}) {
		t.Fatalf("down: %d probes, setup %+v", probes, r.Setup)
	}
	last := log.Recent(1)[0]
	if last.Msg != "service unreachable" || last.Attrs["problem"] != string(probServiceNotInstalled) {
		t.Fatalf("logged %+v", last)
	}
}

func TestMainWindowStartService(t *testing.T) {
	svc := newFakeService()
	svc.down = true
	m, poll := testWindow(t, svc)
	m.poller.probe = func() serviceSetup { return serviceSetup{Known: true, Installed: true, CanStart: true} }
	poll()
	start := button(t, m.w.Content(), "Start the service")
	if start == nil {
		t.Fatalf("no Start the service button: %+v", m.banner.Problem())
	}

	for _, tc := range []struct {
		err    error
		logged string
	}{
		{nil, "service start requested"},
		{errStartCancelled, "service start not allowed"},
		{errors.New("exit status 1"), "couldn't start the service"},
	} {
		calls := 0
		m.startSvc = func(context.Context) error {
			calls++
			return tc.err
		}
		test.Tap(start)
		if calls != 1 || m.starting {
			t.Fatalf("%v: %d calls, starting %v", tc.err, calls, m.starting)
		}
		if got := m.log.Recent(1)[0].Msg; got != tc.logged {
			t.Errorf("%v: logged %q, want %q", tc.err, got, tc.logged)
		}
	}

	// A click while the OS is still asking does nothing.
	m.starting = true
	m.startSvc = func(context.Context) error {
		t.Error("started twice")
		return nil
	}
	test.Tap(start)
	m.starting = false

	svc.set(func(f *fakeService) { f.down = false })
	poll()
	if m.banner.Problem() != nil {
		t.Fatalf("banner after the service started: %+v", m.banner.Problem())
	}
}
