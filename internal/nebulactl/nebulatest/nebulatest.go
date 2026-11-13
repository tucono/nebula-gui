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

// CA is a test certificate authority.
type CA struct {
	crt  cert.Certificate
	priv ed25519.PrivateKey
	// PEM is the CA cert.
	PEM string
}

// NewCA returns a CA valid from notBefore to notAfter. Either may be in the
// past, to build expired fixtures.
func NewCA(notBefore, notAfter time.Time) (*CA, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	crt, err := (&cert.TBSCertificate{
		Version:   cert.Version2,
		Curve:     cert.Curve_CURVE25519,
		Name:      "nebula-gui test CA",
		NotBefore: notBefore.Truncate(time.Second),
		NotAfter:  notAfter.Truncate(time.Second),
		PublicKey: pub,
		IsCA:      true,
	}).Sign(nil, cert.Curve_CURVE25519, priv)
	if err != nil {
		return nil, fmt.Errorf("sign CA: %w", err)
	}
	p, err := crt.MarshalPEM()
	if err != nil {
		return nil, err
	}
	return &CA{crt: crt, priv: priv, PEM: string(p)}, nil
}

// Host signs a host cert for name at addr (for example 192.168.100.20/24),
// valid from notBefore to notAfter, which must lie within the CA's own
// validity. The returned PKI's CA is this CA alone.
func (ca *CA) Host(name, addr string, notBefore, notAfter time.Time, groups ...string) (PKI, error) {
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
		NotBefore: notBefore.Truncate(time.Second),
		NotAfter:  notAfter.Truncate(time.Second),
		PublicKey: pub,
	}).Sign(ca.crt, cert.Curve_CURVE25519, ca.priv)
	if err != nil {
		return PKI{}, fmt.Errorf("sign host cert: %w", err)
	}
	hostPEM, err := host.MarshalPEM()
	if err != nil {
		return PKI{}, err
	}
	return PKI{
		CA:   ca.PEM,
		Cert: string(hostPEM),
		Key:  string(cert.MarshalPrivateKeyToPEM(cert.Curve_CURVE25519, priv)),
	}, nil
}

// NewPKI returns a CA and a host cert for name at addr (for example
// 192.168.100.20/24), valid for a day.
func NewPKI(name, addr string, groups ...string) (PKI, error) {
	now := time.Now()
	ca, err := NewCA(now.Add(-time.Minute), now.Add(48*time.Hour))
	if err != nil {
		return PKI{}, err
	}
	return ca.Host(name, addr, now.Add(-time.Minute), now.Add(24*time.Hour), groups...)
}

// EncryptedKey returns a passphrase-protected key in PEM. Nebula only
// encrypts signing (CA) keys, so this is what a user gets when they're
// handed an encrypted key file.
func EncryptedKey() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	// Small Argon2 parameters: the strength doesn't matter here.
	b, err := cert.EncryptAndMarshalSigningPrivateKey(cert.Curve_CURVE25519, priv, []byte("test"), cert.NewArgon2Parameters(64, 1, 1))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ConfigOptions changes the defaults of Config.
type ConfigOptions struct {
	// ListenPort is the UDP port. 0 picks a random one.
	ListenPort int
	// Tun enables the tun device, which needs privileges.
	Tun bool
}

// Config returns a minimal config for p with the tun device disabled and a
// random listen port, so Nebula can start without privileges.
func (p PKI) Config() string { return p.ConfigWith(ConfigOptions{}) }

// ConfigWith returns a minimal config for p with the given options.
func (p PKI) ConfigWith(o ConfigOptions) string {
	return fmt.Sprintf(`pki:
  ca: |
%s  cert: |
%s  key: |
%s
listen:
  host: 127.0.0.1
  port: %d
tun:
  disabled: %t
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
`, indent(p.CA), indent(p.Cert), indent(p.Key), o.ListenPort, !o.Tun)
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
