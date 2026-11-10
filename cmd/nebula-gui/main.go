// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Command nebula-gui is the desktop app. It runs as the logged-in user and
// controls Nebula through the nebula-guid service.
package main

import (
	"os"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/gui"
	"github.com/tucono/nebula-gui/internal/ipc"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "0.0.0-dev"

// socketEnv overrides the service address, for development.
const socketEnv = "NEBULA_GUI_SOCKET"

func main() {
	addr := os.Getenv(socketEnv)
	if addr == "" {
		addr = ipc.DefaultAddress()
	}
	gui.Run(gui.Options{Client: api.NewClient(addr), AppVersion: version})
}
