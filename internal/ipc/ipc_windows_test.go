// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package ipc

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPipeListenDial(t *testing.T) {
	addr := fmt.Sprintf(`\\.\pipe\nebula-gui-test-%d`, os.Getpid())
	l, err := Listen(addr, ListenOptions{})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		buf := make([]byte, 4)
		n, _ := c.Read(buf)
		_, _ = c.Write(buf[:n])
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx, addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := c.Read(buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}

	if _, err := Listen(addr, ListenOptions{}); err == nil {
		t.Fatal("second Listen on the same pipe name succeeded")
	}
}
