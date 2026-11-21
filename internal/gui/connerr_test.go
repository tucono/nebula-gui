// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

// wrapLikeClient wraps an errno the way a failed dial reaches the GUI:
// os.SyscallError inside net.OpError inside url.Error inside the client's
// "GET /v1/version: %w".
func wrapLikeClient(errno syscall.Errno) error {
	op := &net.OpError{Op: "dial", Net: "unix", Err: os.NewSyscallError("connect", errno)}
	return fmt.Errorf("GET /v1/version: %w", &url.Error{Op: "Get", URL: "http://nebula-guid/v1/version", Err: op})
}

func TestTransportErrnos(t *testing.T) {
	for _, tc := range []struct {
		errnos []syscall.Errno
		want   problemID
	}{
		{notRunningErrnos, probServiceNotRunning},
		{permissionErrnos, probNoPermission},
	} {
		for _, errno := range tc.errnos {
			err := wrapLikeClient(errno)
			p := problemFor(err, problemContext{})
			if p.ID != tc.want {
				t.Errorf("errno %d (%v) = %s, want %s", int(errno), errno, p.ID, tc.want)
			}
			if p.Details != err.Error() {
				t.Errorf("details = %q, want the raw error", p.Details)
			}
		}
	}
}
