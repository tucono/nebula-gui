// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl

import (
	"io"
	"log/slog"
	"testing"
)

func TestDefaultTunDev(t *testing.T) {
	if got := defaultTunDev("windows"); got != windowsTunDev {
		t.Errorf("windows = %q", got)
	}
	for _, goos := range []string{"linux", "darwin", "freebsd"} {
		if got := defaultTunDev(goos); got != "" {
			t.Errorf("%s = %q, want none", goos, got)
		}
	}
}

func TestSetTunDev(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"no tun section", "listen:\n  port: 0\n", "nebula1"},
		{"empty tun section", "tun:\n", "nebula1"},
		{"tun without dev", "tun:\n  mtu: 1300\n", "nebula1"},
		{"empty dev", "tun:\n  dev: \"\"\n", "nebula1"},
		{"dev given", "tun:\n  dev: corp0\n", "corp0"},
		{"tun disabled", "tun:\n  disabled: true\n", ""},
		{"tun not a mapping", "tun: yes\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := load(tc.yaml, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			setTunDev(c, "nebula1")
			if got := c.GetString("tun.dev", ""); got != tc.want {
				t.Fatalf("tun.dev = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("other settings kept", func(t *testing.T) {
		c, err := load("tun:\n  mtu: 1300\n", slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		setTunDev(c, "nebula1")
		if got := c.GetInt("tun.mtu", 0); got != 1300 {
			t.Fatalf("tun.mtu = %d", got)
		}
	})
}
