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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	l, err := ipc.Listen(*listen, ipc.ListenOptions{Owner: *owner, Group: *group})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := service.New(service.Options{Version: version, Logger: logger})
	return srv.Serve(ctx, l)
}
