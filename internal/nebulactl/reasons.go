// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl

import (
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/slackhq/nebula/cert"
	"github.com/slackhq/nebula/util"
)

// Reason says why Check or Start failed, in a form the service can pass on
// without parsing Nebula's messages. See docs/ipc-api.md.
type Reason string

// Reasons that Check and Start can return.
const (
	ReasonCertExpired    Reason = "cert_expired"
	ReasonCAExpired      Reason = "ca_expired"
	ReasonKeyMismatch    Reason = "key_mismatch"
	ReasonWrongCA        Reason = "wrong_ca"
	ReasonKeyEncrypted   Reason = "key_encrypted"
	ReasonTunUnavailable Reason = "tun_unavailable"
	ReasonPortInUse      Reason = "port_in_use"
)

// Error is a failure that Check or Start could classify.
type Error struct {
	Reason Reason
	Err    error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// ReasonOf returns the reason carried by err, or "" if there is none.
func ReasonOf(err error) Reason {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

// checkPKI runs the checks that don't depend on Nebula's wording. It only
// returns errors it can classify: anything it can't parse is left to
// Nebula, which reports it in its own words.
//
// The order matters when several things are wrong: an expired CA means the
// whole network needs new certs, so it is reported before the host cert.
func checkPKI(caPEM, certPEM, keyPEM string, now time.Time) error {
	host, _, err := cert.UnmarshalCertificateFromPEM([]byte(certPEM))
	if err != nil {
		return nil
	}
	cas := parseCAs(caPEM)

	// Nebula refuses a bundle in which every CA has expired.
	if len(cas) > 0 {
		expired := 0
		for _, ca := range cas {
			if ca.Expired(now) {
				expired++
			}
		}
		if expired == len(cas) {
			return &Error{Reason: ReasonCAExpired, Err: errors.New("every CA certificate in pki.ca has expired")}
		}
	}

	if len(cas) > 0 {
		signer := findSigner(cas, host)
		switch {
		case signer == nil:
			return &Error{Reason: ReasonWrongCA, Err: errors.New("pki.cert was not signed by any CA in pki.ca")}
		case signer.NotAfter().Before(now):
			// Other CAs in the bundle may still be valid (during a CA
			// rotation), but this host's cert can't be verified by peers.
			return &Error{Reason: ReasonCAExpired, Err: fmt.Errorf("the CA that signed pki.cert expired on %s", signer.NotAfter().UTC().Format(time.RFC3339))}
		}
	}

	if host.NotAfter().Before(now) {
		return &Error{Reason: ReasonCertExpired, Err: fmt.Errorf("nebula certificate for this host expired on %s", host.NotAfter().UTC().Format(time.RFC3339))}
	}

	if block, _ := pem.Decode([]byte(keyPEM)); block != nil && strings.Contains(block.Type, "ENCRYPTED") {
		return &Error{Reason: ReasonKeyEncrypted, Err: errors.New("pki.key is encrypted and must be decrypted")}
	}
	return nil
}

// parseCAs parses every certificate in a CA bundle, expired ones included
// (Nebula's CA pool drops those, so it can't be used to find the signer).
func parseCAs(bundle string) []cert.Certificate {
	var out []cert.Certificate
	rest := []byte(bundle)
	for len(strings.TrimSpace(string(rest))) > 0 {
		c, r, err := cert.UnmarshalCertificateFromPEM(rest)
		if err != nil {
			if len(r) == len(rest) {
				break // no progress
			}
			rest = r
			continue
		}
		if c.IsCA() {
			out = append(out, c)
		}
		rest = r
	}
	return out
}

// findSigner returns the CA whose fingerprint matches the cert's issuer and
// whose key verifies its signature, or nil.
func findSigner(cas []cert.Certificate, c cert.Certificate) cert.Certificate {
	for _, ca := range cas {
		fp, err := ca.Fingerprint()
		if err != nil || fp != c.Issuer() {
			continue
		}
		if ca.Curve() == c.Curve() && c.CheckSignature(ca.PublicKey()) {
			return ca
		}
	}
	return nil
}

// Messages that Nebula 1.12.0 uses for errors without a sentinel value. The
// tests in this package pin them, so an upgrade that changes one fails a
// test instead of quietly losing the reason.
const (
	msgTunDevice     = "Failed to get a tun/tap device"
	msgInitInterface = "failed to initialize interface"
	msgKeyMismatch   = "private key is not a pair with public key"
	msgAddrInUse     = "address already in use"
)

// classify wraps err from nebula.Main or Control.Start in an *Error when
// its reason can be found. Other errors are returned unchanged.
func classify(err error) error {
	if err == nil || ReasonOf(err) != "" {
		return err
	}
	if r := reasonFor(err); r != "" {
		return &Error{Reason: r, Err: err}
	}
	return err
}

func reasonFor(err error) Reason {
	switch {
	case errors.Is(err, cert.ErrPublicPrivateKeyMismatch),
		errors.Is(err, cert.ErrPublicPrivateCurveMismatch):
		return ReasonKeyMismatch
	case errors.Is(err, cert.ErrCaNotFound), errors.Is(err, cert.ErrSignatureMismatch):
		return ReasonWrongCA
	case errors.Is(err, cert.ErrPrivateKeyEncrypted):
		return ReasonKeyEncrypted
	case isAddrInUse(err):
		return ReasonPortInUse
	}

	var ce *util.ContextualError
	if errors.As(err, &ce) && ce.Context == msgTunDevice {
		return ReasonTunUnavailable
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, msgTunDevice), strings.Contains(msg, msgInitInterface):
		return ReasonTunUnavailable
	case strings.Contains(msg, msgKeyMismatch):
		return ReasonKeyMismatch
	case strings.Contains(msg, msgAddrInUse):
		return ReasonPortInUse
	}
	return ""
}
