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
	// Logs go to stderr and to the ring buffer behind GET /v1/logs.
	logs := service.NewLogBuffer(service.DefaultLogLines)
	logger := slog.New(slog.NewMultiHandler(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}),
		logs.Handler(slog.LevelInfo),
	))
	slog.SetDefault(logger)

	store, err := service.OpenStore(*stateDir)
	if err != nil {
		return err
	}
	srv, err := service.New(service.Options{
		Version: version,
		Logger:  logger,
		Engine:  service.NebulaEngine{BuildVersion: version, Logger: logger},
		Store:   store,
		Logs:    logs,
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
