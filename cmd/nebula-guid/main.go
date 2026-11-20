// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Command nebula-guid is the privileged service that runs Nebula and serves
// the API the GUI talks to.
//
// With no command it runs in the foreground: that's how systemd and launchd
// run it, and how it runs during development. On Windows the commands
// install, uninstall and run register it with the Service Control Manager
// and run under it. On every system, remove-user-files removes each user's
// nebula-gui files for the uninstallers.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/tucono/nebula-gui/internal/ipc"
	"github.com/tucono/nebula-gui/internal/service"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "0.0.0-dev"

func main() {
	err := run(os.Args[1:])
	switch {
	case err == nil:
	case errors.Is(err, flag.ErrHelp):
		// The flag package has printed the usage.
	default:
		fmt.Fprintln(os.Stderr, "nebula-guid:", err)
		os.Exit(1)
	}
}

// run dispatches on the command, if there is one.
func run(args []string) error {
	cmd, rest, err := splitCommand(args)
	if err != nil {
		return err
	}
	switch cmd {
	case "install":
		return installService(rest)
	case "uninstall":
		return uninstallService(rest)
	case "run":
		return runService(rest)
	case "remove-user-files":
		return removeUserFiles(rest)
	default:
		return foreground(rest)
	}
}

// splitCommand separates a leading command from the flags after it. An
// empty command means none was given: args start with a flag, or are empty.
func splitCommand(args []string) (cmd string, rest []string, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", args, nil
	}
	switch args[0] {
	case "install", "uninstall", "run", "remove-user-files":
		return args[0], args[1:], nil
	}
	return "", nil, fmt.Errorf("unknown command %q (commands: install, uninstall, run, remove-user-files)", args[0])
}

// options are the flags for running the service, in the foreground or
// under the SCM.
type options struct {
	listen   string
	owner    string
	group    string
	stateDir string
	dev      bool
	version  bool
}

// parseOptions parses the service flags. name is the command, for usage
// messages.
func parseOptions(name string, args []string) (*options, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	o := &options{}
	fs.StringVar(&o.listen, "listen", ipc.DefaultAddress(), "Unix socket path or Windows named pipe to listen on")
	fs.StringVar(&o.owner, "socket-owner", "", "user given access to the socket or pipe")
	fs.StringVar(&o.group, "socket-group", "", "group given access to the socket or pipe")
	fs.StringVar(&o.stateDir, "state-dir", service.DefaultStateDir(), "directory for the imported config (created 0700)")
	dev := devFlag(fs)
	fs.BoolVar(&o.version, "version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	o.dev = *dev
	if o.dev && o.owner == "" {
		o.owner = devSocketOwner()
	}
	return o, nil
}

// foreground runs the service until SIGINT or SIGTERM.
func foreground(args []string) error {
	o, err := parseOptions("nebula-guid", args)
	if err != nil {
		return err
	}
	if o.version {
		fmt.Println(version)
		return nil
	}
	if err := checkPrivileges(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, o)
}

// checkPrivileges stops a release build early when it can't create the
// Nebula interface, with a message that says why, rather than leaving it to
// fail on the first connect. Dev builds skip it: make dev-service grants
// the capabilities itself.
func checkPrivileges() error {
	if devBuild {
		return nil
	}
	return checkNetAdmin()
}

// serve runs the service until ctx ends.
func serve(ctx context.Context, o *options) error {
	level := slog.LevelInfo
	if o.dev {
		level = slog.LevelDebug
	}
	// The store creates and restricts the state directory, which holds the
	// log directory too.
	store, err := service.OpenStore(o.stateDir)
	if err != nil {
		return err
	}

	// Logs go to stderr, to the ring buffer behind GET /v1/logs, and to the
	// log file behind GET /v1/diagnostics. A log file that can't be opened
	// must never stop the VPN, so the service runs on without it.
	logs := service.NewLogBuffer(service.DefaultLogLines)
	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}),
		logs.Handler(slog.LevelInfo),
	}
	logFile, err := service.OpenLogFile(filepath.Join(store.Dir(), "logs"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "nebula-guid: warning: running without a log file:", err)
	} else {
		defer func() { _ = logFile.Close() }()
		handlers = append(handlers, slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	logger := slog.New(slog.NewMultiHandler(handlers...))
	slog.SetDefault(logger)

	srv, err := service.New(service.Options{
		Version: version,
		Logger:  logger,
		Engine:  service.NebulaEngine{BuildVersion: version, Logger: logger},
		Store:   store,
		Logs:    logs,
		LogFile: logFile,
	})
	if err != nil {
		return err
	}

	l, err := ipc.Listen(o.listen, ipc.ListenOptions{Owner: o.owner, Group: o.group})
	if err != nil {
		return err
	}
	return srv.Serve(ctx, l)
}
