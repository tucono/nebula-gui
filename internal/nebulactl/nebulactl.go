// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package nebulactl is a thin wrapper over the embedded Nebula library. It is
// the only package that imports github.com/slackhq/nebula, so Nebula API
// changes between versions stay in one place. No Nebula types leak out of it.
package nebulactl

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/slackhq/nebula"
	"github.com/slackhq/nebula/cert"
	"github.com/slackhq/nebula/config"
)

const modulePath = "github.com/slackhq/nebula"

// Version returns the version of the embedded Nebula module, or "" if build
// info is unavailable.
func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, m := range bi.Deps {
		if m.Path == modulePath {
			if m.Replace != nil && m.Replace.Version != "" {
				return strings.TrimPrefix(m.Replace.Version, "v")
			}
			return strings.TrimPrefix(m.Version, "v")
		}
	}
	return ""
}

// CertInfo is the host certificate's display data.
type CertInfo struct {
	Name        string
	Groups      []string
	Networks    []netip.Prefix
	NotAfter    time.Time
	Fingerprint string
}

// Info is what the service needs to know about a config without running it.
type Info struct {
	Cert       CertInfo
	ListenPort int
}

// Check loads raw (a full Nebula config with inline PKI) and runs Nebula's
// own config test, which builds everything except the tun device and UDP
// sockets. It returns the host cert's details.
//
// The config test logs the whole config, private key included, at info
// level, so Nebula's output is discarded here.
func Check(raw string) (*Info, error) {
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	c, err := load(raw, l)
	if err != nil {
		return nil, err
	}
	if _, err := nebula.Main(c, true, "", l, nil); err != nil {
		return nil, err
	}
	ci, err := ParseCert(c.GetString("pki.cert", ""))
	if err != nil {
		return nil, err
	}
	return &Info{Cert: *ci, ListenPort: c.GetInt("listen.port", 0)}, nil
}

// ParseCert parses the first certificate in a PEM string.
func ParseCert(pemStr string) (*CertInfo, error) {
	crt, _, err := cert.UnmarshalCertificateFromPEM([]byte(pemStr))
	if err != nil {
		return nil, fmt.Errorf("parse pki.cert: %w", err)
	}
	fp, err := crt.Fingerprint()
	if err != nil {
		return nil, fmt.Errorf("fingerprint pki.cert: %w", err)
	}
	return &CertInfo{
		Name:        crt.Name(),
		Groups:      append([]string{}, crt.Groups()...),
		Networks:    append([]netip.Prefix{}, crt.Networks()...),
		NotAfter:    crt.NotAfter(),
		Fingerprint: fp,
	}, nil
}

func load(raw string, l *slog.Logger) (*config.C, error) {
	c := config.NewC(l)
	if err := c.LoadString(raw); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return c, nil
}

// stopTimeout bounds how long Stop waits for Nebula's packet readers to
// exit after the teardown itself has finished.
const stopTimeout = 10 * time.Second

// Instance is a running Nebula.
type Instance struct {
	ctrl *nebula.Control
	log  *slog.Logger
	done chan struct{}

	stopOnce sync.Once
	err      error // set before done is closed
}

// Start runs Nebula with raw, a full config with inline PKI. It creates the
// tun device and UDP listeners, so it needs privileges. Nebula logs to l.
func Start(raw, buildVersion string, l *slog.Logger) (*Instance, error) {
	c, err := load(raw, l)
	if err != nil {
		return nil, err
	}
	ctrl, err := nebula.Main(c, false, buildVersion, l, nil)
	if err != nil {
		return nil, err
	}
	if err := ctrl.Start(); err != nil {
		ctrl.Stop()
		return nil, fmt.Errorf("start nebula: %w", err)
	}
	inst := &Instance{ctrl: ctrl, log: l, done: make(chan struct{})}
	go func() {
		inst.err = ctrl.Wait()
		close(inst.done)
	}()
	return inst, nil
}

// Stop shuts Nebula down and waits until it has stopped. Control.Stop does
// the teardown (tunnels, tun device, sockets) synchronously; the wait after
// it is for the packet reader goroutines, and is bounded because a reader
// that hadn't started yet can block forever (seen with tun.disabled in
// Nebula 1.12.0).
func (i *Instance) Stop() {
	i.stopOnce.Do(i.ctrl.Stop)
	select {
	case <-i.done:
	case <-time.After(stopTimeout):
		i.log.Warn("nebula packet readers did not exit after stop")
	}
}

// Done is closed once Nebula has stopped, whether through Stop or a fatal
// error.
func (i *Instance) Done() <-chan struct{} { return i.done }

// Err returns the fatal error that stopped Nebula, if any. It is valid only
// after Done is closed.
func (i *Instance) Err() error {
	select {
	case <-i.done:
		return i.err
	default:
		return nil
	}
}

// HostInfo is one entry in Nebula's hostmap.
type HostInfo struct {
	VpnAddrs      []netip.Addr
	Name          string
	Groups        []string
	CertNotAfter  time.Time
	CurrentRemote netip.AddrPort
	RemoteAddrs   []netip.AddrPort
	Relayed       bool
}

// Hosts lists established tunnels, or pending handshakes if pending is set.
func (i *Instance) Hosts(pending bool) []HostInfo {
	hosts := i.ctrl.ListHostmapHosts(pending)
	out := make([]HostInfo, 0, len(hosts))
	for _, h := range hosts {
		hi := HostInfo{
			VpnAddrs:      h.VpnAddrs,
			CurrentRemote: h.CurrentRemote,
			RemoteAddrs:   h.RemoteAddrs,
			Relayed:       !h.CurrentRemote.IsValid() && len(h.CurrentRelaysToMe) > 0,
		}
		if h.Cert != nil {
			hi.Name = h.Cert.Name()
			hi.Groups = h.Cert.Groups()
			hi.CertNotAfter = h.Cert.NotAfter()
		}
		out = append(out, hi)
	}
	return out
}

// ErrNoTunnel is returned by CloseTunnel when there is no tunnel to addr.
var ErrNoTunnel = errors.New("no tunnel to that address")

// CloseTunnel closes the tunnel to addr and tells the peer.
func (i *Instance) CloseTunnel(addr netip.Addr) error {
	if !i.ctrl.CloseTunnel(addr, false) {
		return ErrNoTunnel
	}
	return nil
}
