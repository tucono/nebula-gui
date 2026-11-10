// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build unix

package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Listen creates a Unix socket at addr.
//
// The socket is created inside a private (0700) temporary directory, given
// its final owner, group and mode, and only then renamed to addr. Nobody can
// connect before the permissions are in place.
//
// A stale socket left at addr by a crashed service is removed. If a service
// is still answering there, Listen returns ErrInUse. Closing the listener
// removes the socket.
func Listen(addr string, opts ListenOptions) (net.Listener, error) {
	uid, gid, err := lookupIDs(opts)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(addr)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the socket's own mode restricts access
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if err := removeStale(addr); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp(dir, ".nebula-guid-")
	if err != nil {
		return nil, fmt.Errorf("create temporary socket directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmp := filepath.Join(tmpDir, "s")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: tmp, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	l.SetUnlinkOnClose(false)

	mode := os.FileMode(0o600)
	if opts.Group != "" {
		mode = 0o660
	}
	if err := setup(tmp, addr, uid, gid, mode); err != nil {
		_ = l.Close()
		return nil, err
	}
	return &unixListener{UnixListener: l, path: addr}, nil
}

func setup(tmp, addr string, uid, gid int, mode os.FileMode) error {
	if uid != -1 || gid != -1 {
		if err := os.Chown(tmp, uid, gid); err != nil {
			return fmt.Errorf("set socket owner: %w", err)
		}
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return fmt.Errorf("set socket mode: %w", err)
	}
	if err := os.Rename(tmp, addr); err != nil {
		return fmt.Errorf("move socket into place: %w", err)
	}
	return nil
}

func lookupIDs(opts ListenOptions) (uid, gid int, err error) {
	uid, gid = -1, -1
	if opts.Owner != "" {
		u, err := user.Lookup(opts.Owner)
		if err != nil {
			return 0, 0, fmt.Errorf("socket owner: %w", err)
		}
		if uid, err = strconv.Atoi(u.Uid); err != nil {
			return 0, 0, fmt.Errorf("socket owner %q: bad uid %q", opts.Owner, u.Uid)
		}
	}
	if opts.Group != "" {
		g, err := user.LookupGroup(opts.Group)
		if err != nil {
			return 0, 0, fmt.Errorf("socket group: %w", err)
		}
		if gid, err = strconv.Atoi(g.Gid); err != nil {
			return 0, 0, fmt.Errorf("socket group %q: bad gid %q", opts.Group, g.Gid)
		}
	}
	return uid, gid, nil
}

// removeStale deletes a socket at addr that nothing is listening on.
func removeStale(addr string) error {
	fi, err := os.Lstat(addr)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check socket path: %w", err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket", addr)
	}
	conn, err := net.DialTimeout("unix", addr, time.Second)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("%s: %w", addr, ErrInUse)
	}
	if err := os.Remove(addr); err != nil {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}

// unixListener removes the socket at its final path on Close.
type unixListener struct {
	*net.UnixListener
	path string
	once sync.Once
}

// Addr reports the final socket path, not the temporary one it was bound to.
func (l *unixListener) Addr() net.Addr {
	return &net.UnixAddr{Name: l.path, Net: "unix"}
}

func (l *unixListener) Close() error {
	err := l.UnixListener.Close()
	l.once.Do(func() { _ = os.Remove(l.path) })
	return err
}

// Dial connects to the service socket at addr.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}
