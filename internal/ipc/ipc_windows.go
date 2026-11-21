// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package ipc

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

// baseSDDL is a protected DACL that grants full access to SYSTEM and
// Administrators only. Listen appends entries for opts.Owner and opts.Group.
const baseSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)"

// Listen creates a named pipe at addr (for example \\.\pipe\nebula-guid)
// with an explicit security descriptor. It fails if another process already
// owns the pipe name.
func Listen(addr string, opts ListenOptions) (net.Listener, error) {
	sddl, err := pipeSDDL(opts)
	if err != nil {
		return nil, err
	}
	l, err := winio.ListenPipe(addr, &winio.PipeConfig{SecurityDescriptor: sddl})
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return l, nil
}

func pipeSDDL(opts ListenOptions) (string, error) {
	sddl := baseSDDL
	for _, name := range []string{opts.Owner, opts.Group} {
		if name == "" {
			continue
		}
		sid, err := winio.LookupSidByName(name)
		if err != nil {
			return "", fmt.Errorf("look up %q: %w", name, err)
		}
		sddl += "(A;;GRGW;;;" + sid + ")"
	}
	return sddl, nil
}

// Dial connects to the service pipe at addr.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, addr)
}
