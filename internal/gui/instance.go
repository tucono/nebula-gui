// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/tucono/nebula-gui/internal/ipc"
)

// One copy of the app runs per user. The first listens on a per-user
// socket or pipe (see instance_unix.go and instance_windows.go); a later
// copy finds it there, asks it to show its window, and exits.

// errAlreadyRunning is returned by claimInstance when another copy answered.
var errAlreadyRunning = errors.New("nebula-gui is already running")

const (
	// showRequest is the only message. Anything else is ignored, so the
	// socket can't be used to drive the app.
	showRequest = "show"
	showReply   = "ok"
	// instanceWait bounds each step of talking to the other copy.
	instanceWait = 2 * time.Second
)

// instance is this copy's claim on the address.
type instance struct {
	l    net.Listener
	once sync.Once
	done chan struct{}
}

// claimInstance makes this the running copy, or, if another copy already
// answers at addr, returns errAlreadyRunning after asking it to show its
// window. With show false (starting hidden at sign-in) the other copy is
// left as it is.
func claimInstance(addr string, show bool) (*instance, error) {
	if askRunning(addr, show) == nil {
		return nil, errAlreadyRunning
	}
	l, err := listenInstance(addr)
	if err != nil {
		// Another copy may have started between the two steps.
		if askRunning(addr, show) == nil {
			return nil, errAlreadyRunning
		}
		return nil, err
	}
	return &instance{l: l, done: make(chan struct{})}, nil
}

// askRunning reaches the copy at addr and, with show, has it show its
// window. It fails if nothing answers.
func askRunning(addr string, show bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), instanceWait)
	defer cancel()
	c, err := ipc.Dial(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if !show {
		return nil
	}
	_ = c.SetDeadline(time.Now().Add(instanceWait))
	if _, err := c.Write([]byte(showRequest + "\n")); err != nil {
		return err
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(line) != showReply {
		return errors.New("unexpected reply from the running copy")
	}
	return nil
}

// serve answers later copies until Close, calling onShow for each show
// request. onShow runs on serve's goroutine.
func (in *instance) serve(onShow func()) {
	for {
		c, err := in.l.Accept()
		if err != nil {
			select {
			case <-in.done:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(100 * time.Millisecond) // don't spin on a transient error
			continue
		}
		go handleInstanceConn(c, onShow)
	}
}

func handleInstanceConn(c net.Conn, onShow func()) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(instanceWait))
	// A show request is one short line; read no more than that.
	line, err := bufio.NewReaderSize(c, 16).ReadSlice('\n')
	if err != nil || strings.TrimSpace(string(line)) != showRequest {
		return
	}
	onShow()
	_, _ = c.Write([]byte(showReply + "\n"))
}

// Close stops answering and frees the address for the next copy.
func (in *instance) Close() error {
	var err error
	in.once.Do(func() {
		close(in.done)
		err = in.l.Close()
	})
	return err
}
