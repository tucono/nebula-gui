// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package nebulatest builds throwaway Nebula configs for tests. Every call
// generates a fresh CA and host cert in memory, so no key material is ever
// written to the repo.
package nebulatest

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/slackhq/nebula/cert"
	"golang.org/x/crypto/curve25519"
)

// PKI is an inline CA, host cert and host key, all PEM.
type PKI struct {
	CA, Cert, Key string
}

// NewPKI returns a CA and a host cert for name at addr (for example
// 192.168.100.20/24), valid for a day.
func NewPKI(name, addr string, groups ...string) (PKI, error) {
	now := time.Now().Truncate(time.Second)
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PKI{}, err
	}
	ca, err := (&cert.TBSCertificate{
		Version:   cert.Version2,
		Curve:     cert.Curve_CURVE25519,
		Name:      "nebula-gui test CA",
		NotBefore: now.Add(-time.Minute),
		NotAfter:  now.Add(48 * time.Hour),
		PublicKey: caPub,
		IsCA:      true,
	}).Sign(nil, cert.Curve_CURVE25519, caPriv)
	if err != nil {
		return PKI{}, fmt.Errorf("sign CA: %w", err)
	}

	prefix, err := netip.ParsePrefix(addr)
	if err != nil {
		return PKI{}, err
	}
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return PKI{}, err
	}
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		return PKI{}, err
	}
	host, err := (&cert.TBSCertificate{
		Version:   cert.Version2,
		Curve:     cert.Curve_CURVE25519,
		Name:      name,
		Networks:  []netip.Prefix{prefix},
		Groups:    groups,
		NotBefore: now.Add(-time.Minute),
		NotAfter:  now.Add(24 * time.Hour),
		PublicKey: pub,
	}).Sign(ca, cert.Curve_CURVE25519, caPriv)
	if err != nil {
		return PKI{}, fmt.Errorf("sign host cert: %w", err)
	}

	caPEM, err := ca.MarshalPEM()
	if err != nil {
		return PKI{}, err
	}
	hostPEM, err := host.MarshalPEM()
	if err != nil {
		return PKI{}, err
	}
	return PKI{
		CA:   string(caPEM),
		Cert: string(hostPEM),
		Key:  string(cert.MarshalPrivateKeyToPEM(cert.Curve_CURVE25519, priv)),
	}, nil
}

// Config returns a minimal config for p with the tun device disabled and a
// random listen port, so Nebula can start without privileges.
func (p PKI) Config() string {
	return fmt.Sprintf(`pki:
  ca: |
%s  cert: |
%s  key: |
%s
listen:
  host: 127.0.0.1
  port: 0
tun:
  disabled: true
lighthouse:
  am_lighthouse: false
firewall:
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - proto: icmp
      host: any
`, indent(p.CA), indent(p.Cert), indent(p.Key))
}

func indent(s string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		if line == "" {
			continue
		}
		b.WriteString("    ")
		b.WriteString(line)
	}
	return b.String()
}

// SkipIfRace skips tests that run a real Nebula with tun.disabled under the
// race detector: Nebula 1.12.0's disabled tun device closes its channel
// while a reader goroutine reads the field (overlay/tun_disabled.go), which
// the detector reports. Real tun devices don't use that code.
func SkipIfRace(t interface{ Skip(...any) }) {
	if raceEnabled {
		t.Skip("Nebula 1.12.0's disabled tun device has a data race on close")
	}
}
