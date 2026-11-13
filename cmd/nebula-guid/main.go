// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Command nebula-guid is the privileged service that runs Nebula and serves
// the API the GUI talks to.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/tucono/nebula-gui/internal/ipc"
	"github.com/tucono/nebula-gui/internal/service"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "0.0.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nebula-guid:", err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", ipc.DefaultAddress(), "Unix socket path or Windows named pipe to listen on")
	owner := flag.String("socket-owner", "", "user given access to the socket or pipe")
	group := flag.String("socket-group", "", "group given access to the socket or pipe")
	stateDir := flag.String("state-dir", service.DefaultStateDir(), "directory for the imported config (created 0700)")
	dev := devFlag()
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	level := slog.LevelInfo
	if *dev {
		level = slog.LevelDebug
		if *owner == "" {
			*owner = devSocketOwner()
		}
	}
	// The store creates and restricts the state directory, which holds the
	// log directory too.
	store, err := service.OpenStore(*stateDir)
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

	l, err := ipc.Listen(*listen, ipc.ListenOptions{Owner: *owner, Group: *group})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return srv.Serve(ctx, l)
}
