// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix

package gui

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tucono/nebula-gui/internal/ipc"
	"github.com/tucono/nebula-gui/internal/userfiles"
)

// defaultInstanceAddr is the per-user socket: in $XDG_RUNTIME_DIR on Linux,
// and in the user's cache directory on macOS or when there's no runtime
// directory.
func defaultInstanceAddr() (string, error) {
	if base := os.Getenv("XDG_RUNTIME_DIR"); runtime.GOOS != "darwin" && base != "" {
		return filepath.Join(base, userfiles.DirName, "gui.sock"), nil
	}
	dir, err := userfiles.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gui.sock"), nil
}

// listenInstance listens on a 0600 socket at addr, in a directory only this
// user can enter. A stale socket from a crash is removed.
func listenInstance(addr string) (net.Listener, error) {
	dir := filepath.Dir(addr)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create instance directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // a directory needs its search bit
		return nil, fmt.Errorf("restrict instance directory: %w", err)
	}
	return ipc.Listen(addr, ipc.ListenOptions{})
}
