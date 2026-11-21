// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// defaultInstanceAddr is a pipe named after the user's SID, so each user
// on the machine has their own.
func defaultInstanceAddr() (string, error) {
	sid, err := currentUserSID()
	if err != nil {
		return "", err
	}
	return `\\.\pipe\` + appName + "-" + sid, nil
}

func currentUserSID() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("look up the current user: %w", err)
	}
	return u.User.Sid.String(), nil
}

// instanceSDDL lets only the given user open the pipe: not SYSTEM or
// Administrators, unlike the service's pipe.
func instanceSDDL(sid string) string { return "D:P(A;;GA;;;" + sid + ")" }

// listenInstance listens on the pipe at addr. It fails if the name is
// already taken.
func listenInstance(addr string) (net.Listener, error) {
	sid, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	l, err := winio.ListenPipe(addr, &winio.PipeConfig{SecurityDescriptor: instanceSDDL(sid)})
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	return l, nil
}
