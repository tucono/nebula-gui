// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName        = "nebula-guid"
	serviceDisplayName = "nebula-gui service"
	serviceDescription = "Runs Nebula for the nebula-gui app."
	// socketGroup is the local group whose members may use the app. The
	// installer creates it.
	socketGroup = "nebula-gui"
	// stopTimeout is how long uninstall waits for the service to stop.
	stopTimeout = 30 * time.Second
	// eventID is the ID of every Event Log entry. Sources registered with
	// InstallAsEventCreate accept 1 to 1000.
	eventID = 1
)

// serviceArgs is the command line the SCM starts the service with.
func serviceArgs(allowUser string) []string {
	args := []string{"run", "--socket-group", socketGroup}
	if allowUser != "" {
		args = append(args, "--socket-owner", allowUser)
	}
	return args
}

// installFlags are the install command's flags.
type installFlags struct {
	allowUser   string
	sessionUser bool
}

// parseInstall parses the flags of the install command.
func parseInstall(args []string) (installFlags, error) {
	var f installFlags
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.StringVar(&f.allowUser, "allow-user", "",
		"user given access to the service at once, and added to the "+socketGroup+" group")
	fs.BoolVar(&f.sessionUser, "allow-session-user", false,
		"like --allow-user, for the person signed in to this session (what the installer uses)")
	if err := fs.Parse(args); err != nil {
		return installFlags{}, err
	}
	if fs.NArg() > 0 {
		return installFlags{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if f.allowUser != "" && f.sessionUser {
		return installFlags{}, errors.New("use --allow-user or --allow-session-user, not both")
	}
	return f, nil
}

// installService creates the group, gives the allowed user access, and
// registers the service: automatic start, LocalSystem, restarted on
// failure. It doesn't start it.
func installService(args []string) error {
	f, err := parseInstall(args)
	if err != nil {
		return err
	}
	if err := ensureGroup(socketGroup, "People who may use the nebula-gui app"); err != nil {
		return err
	}
	allowUser := f.allowUser
	if f.sessionUser {
		allowUser, err = sessionUser()
		if errors.Is(err, errNoSessionUser) {
			fmt.Printf("%v; add each person who'll use the app to the %s group.\n", err, socketGroup)
		} else if err != nil {
			return err
		}
	}
	if allowUser != "" {
		// The service looks it up every time it starts; catch a typo now
		// rather than as a service that won't start.
		if _, err := winio.LookupSidByName(allowUser); err != nil {
			return fmt.Errorf("no user %q: %w", allowUser, err)
		}
		// Direct access works at once; the group, after the next sign-in,
		// and it outlasts a reinstall.
		if err := addGroupMember(socketGroup, allowUser); err != nil {
			return err
		}
		fmt.Printf("Gave %s access, and added them to the %s group.\n", allowUser, socketGroup)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to the service manager: %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	if s, err := m.OpenService(serviceName); err == nil {
		_ = s.Close()
		return fmt.Errorf("the %s service is already installed", serviceName)
	}
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
	}, serviceArgs(allowUser)...)
	if err != nil {
		return fmt.Errorf("create the service: %w", err)
	}
	defer func() { _ = s.Close() }()

	if err := setupService(s); err != nil {
		_ = s.Delete()
		return err
	}
	fmt.Printf("Installed the %s service. Start it with: sc start %s\n", serviceName, serviceName)
	return nil
}

// setupService sets the restart policy and the Event Log source.
func setupService(s *mgr.Service) error {
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 5 * time.Second}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, 24*60*60); err != nil {
		return fmt.Errorf("set the restart policy: %w", err)
	}
	// Execute reports a failure with a non-zero exit code rather than a
	// crash; restart then too.
	if err := s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("set the restart policy: %w", err)
	}
	// A source left behind by an interrupted uninstall would make the
	// install fail.
	_ = eventlog.Remove(serviceName)
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		return fmt.Errorf("register the Event Log source: %w", err)
	}
	return nil
}

// uninstallService stops the service, which stops Nebula, and removes it.
// The state directory stays.
func uninstallService(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to the service manager: %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("the %s service isn't installed: %w", serviceName, err)
	}
	defer func() { _ = s.Close() }()

	stopErr := stopService(s)
	if err := s.Delete(); err != nil {
		return fmt.Errorf("remove the service: %w", err)
	}
	_ = eventlog.Remove(serviceName)
	if stopErr != nil {
		// Delete only marks it; the SCM removes it once it stops.
		return fmt.Errorf("%w; it will be removed when it stops", stopErr)
	}
	fmt.Printf("Removed the %s service.\n", serviceName)
	return nil
}

// stopService asks the service to stop and waits up to stopTimeout.
func stopService(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil {
		return fmt.Errorf("query the service: %w", err)
	}
	if st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return fmt.Errorf("stop the service: %w", err)
	}
	deadline := time.Now().Add(stopTimeout)
	for {
		st, err := s.Query()
		if err != nil {
			return fmt.Errorf("query the service: %w", err)
		}
		if st.State == svc.Stopped {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the service didn't stop within %s", stopTimeout)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// runService is what the SCM starts. Run from a console, it runs in the
// foreground like no command at all, which helps when testing the
// installed command line.
func runService(args []string) error {
	inService, err := svc.IsWindowsService()
	if err != nil {
		return fmt.Errorf("check for the service manager: %w", err)
	}
	if !inService {
		return foreground(args)
	}

	elog := openEventLog()
	defer func() { _ = elog.Close() }()
	o, err := parseOptions("run", args)
	if err == nil && o.version {
		err = errors.New("--version can't be used in the service")
	}
	if err != nil {
		_ = elog.Error(eventID, "nebula-guid can't start: "+err.Error())
		return err
	}
	h := &handler{serve: func(ctx context.Context) error { return serve(ctx, o) }, elog: elog}
	if err := svc.Run(serviceName, h); err != nil {
		_ = elog.Error(eventID, "nebula-guid failed: "+err.Error())
		return err
	}
	return nil
}

// eventLog is the part of *eventlog.Log the service uses.
type eventLog interface {
	Info(eid uint32, msg string) error
	Error(eid uint32, msg string) error
	Close() error
}

// openEventLog opens the service's Event Log source. Logging there is a
// courtesy; without it the service runs anyway.
func openEventLog() eventLog {
	l, err := eventlog.Open(serviceName)
	if err != nil {
		return nopEventLog{}
	}
	return l
}

type nopEventLog struct{}

func (nopEventLog) Info(uint32, string) error  { return nil }
func (nopEventLog) Error(uint32, string) error { return nil }
func (nopEventLog) Close() error               { return nil }

// handler runs serve under the SCM. A Stop or Shutdown request cancels
// serve's context, so Nebula stops cleanly before the service reports
// stopped.
type handler struct {
	serve func(ctx context.Context) error
	elog  eventLog
}

// exitServeFailed is the service-specific exit code when serve ends
// without being asked to. It triggers the restart policy.
const exitServeFailed = 1

// stopWaitHint is how long the SCM is told a stop may take.
const stopWaitHint = 30 * time.Second

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.serve(ctx) }()
	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	_ = h.elog.Info(eventID, "nebula-guid started, version "+version)

	for {
		select {
		case err := <-done:
			// Nothing asked it to stop: a failure, such as the pipe
			// being taken.
			if err == nil {
				err = errors.New("stopped without being asked to")
			}
			_ = h.elog.Error(eventID, "nebula-guid failed: "+err.Error())
			return true, exitServeFailed
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending, WaitHint: uint32(stopWaitHint / time.Millisecond)}
				cancel()
				// It was asked to stop, so it reports success even if
				// stopping went wrong: a failure would restart it.
				if err := <-done; err != nil {
					_ = h.elog.Error(eventID, "nebula-guid stopped with an error: "+err.Error())
				} else {
					_ = h.elog.Info(eventID, "nebula-guid stopped")
				}
				return false, 0
			}
		}
	}
}
