// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl_test

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/nebulactl"
	"github.com/tucono/nebula-gui/internal/nebulactl/nebulatest"
)

const (
	hour = time.Hour
	day  = 24 * time.Hour
)

func newCA(t *testing.T, from, until time.Duration) *nebulatest.CA {
	t.Helper()
	now := time.Now()
	ca, err := nebulatest.NewCA(now.Add(from), now.Add(until))
	if err != nil {
		t.Fatal(err)
	}
	return ca
}

func host(t *testing.T, ca *nebulatest.CA, from, until time.Duration) nebulatest.PKI {
	t.Helper()
	now := time.Now()
	p, err := ca.Host("dev", "192.168.100.20/24", now.Add(from), now.Add(until), "users")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// pkiFixtures returns one config per reason that Check and Start find in
// the PKI, all built from real certs.
func pkiFixtures(t *testing.T) map[string]struct {
	pki  nebulatest.PKI
	want nebulactl.Reason
} {
	t.Helper()
	valid := newCA(t, -3*day, 3*day)
	expiredCA := newCA(t, -3*day, -hour)

	expiredCert := host(t, valid, -2*day, -hour)

	caExpired := host(t, expiredCA, -2*day, -2*hour)

	// A bundle during a CA rotation: the old CA has expired, the new one
	// is valid.
	signedByNew := host(t, valid, -hour, day)
	signedByNew.CA = expiredCA.PEM + valid.PEM
	signedByOld := host(t, expiredCA, -2*day, -2*hour)
	signedByOld.CA = expiredCA.PEM + valid.PEM

	mismatched := host(t, valid, -hour, day)
	mismatched.Key = host(t, valid, -hour, day).Key

	wrongCA := host(t, valid, -hour, day)
	wrongCA.CA = newCA(t, -day, day).PEM

	encrypted := host(t, valid, -hour, day)
	key, err := nebulatest.EncryptedKey()
	if err != nil {
		t.Fatal(err)
	}
	encrypted.Key = key

	return map[string]struct {
		pki  nebulatest.PKI
		want nebulactl.Reason
	}{
		"expired host cert":                {expiredCert, nebulactl.ReasonCertExpired},
		"expired CA":                       {caExpired, nebulactl.ReasonCAExpired},
		"bundle, signed by the valid CA":   {signedByNew, ""},
		"bundle, signed by the expired CA": {signedByOld, nebulactl.ReasonCAExpired},
		"mismatched key":                   {mismatched, nebulactl.ReasonKeyMismatch},
		"cert from another CA":             {wrongCA, nebulactl.ReasonWrongCA},
		"encrypted key":                    {encrypted, nebulactl.ReasonKeyEncrypted},
	}
}

func TestCheckReasons(t *testing.T) {
	for name, tc := range pkiFixtures(t) {
		t.Run(name, func(t *testing.T) {
			info, err := nebulactl.Check(tc.pki.Config())
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Check: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Check accepted the config, want %s", tc.want)
			}
			if got := nebulactl.ReasonOf(err); got != tc.want {
				t.Fatalf("reason = %q, want %q (err: %v)", got, tc.want, err)
			}
			if info == nil || info.Cert.Name != "dev" {
				t.Errorf("info = %+v, want the cert described despite the error", info)
			}
		})
	}
}

func TestStartReasons(t *testing.T) {
	l := slog.New(slog.NewTextHandler(io.Discard, nil))
	for name, tc := range pkiFixtures(t) {
		if tc.want == "" {
			continue // a valid config would start Nebula
		}
		t.Run(name, func(t *testing.T) {
			inst, err := nebulactl.Start(tc.pki.Config(), "test", l)
			if err == nil {
				inst.Stop()
				t.Fatalf("Start accepted the config, want %s", tc.want)
			}
			if got := nebulactl.ReasonOf(err); got != tc.want {
				t.Fatalf("reason = %q, want %q (err: %v)", got, tc.want, err)
			}
		})
	}
}

// CANotAfter comes from the CA that signed the cert, and is zero when
// that CA isn't in the bundle.
func TestCheckCANotAfter(t *testing.T) {
	ca := newCA(t, -3*day, 3*day)
	p := host(t, ca, -hour, day)
	info, err := nebulactl.Check(p.Config())
	if err != nil {
		t.Fatal(err)
	}
	if info.Cert.CANotAfter.IsZero() || info.Cert.CANotAfter.Before(info.Cert.NotAfter) {
		t.Errorf("CANotAfter = %v, cert NotAfter = %v", info.Cert.CANotAfter, info.Cert.NotAfter)
	}
	p.CA = newCA(t, -day, day).PEM
	if info, _ := nebulactl.Check(p.Config()); info == nil || !info.Cert.CANotAfter.IsZero() {
		t.Errorf("info = %+v, want CANotAfter zero when the signer isn't in pki.ca", info)
	}
}

func TestStartPortInUse(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	port := pc.LocalAddr().(*net.UDPAddr).Port

	cfg := newPKI(t).ConfigWith(nebulatest.ConfigOptions{ListenPort: port})
	inst, err := nebulactl.Start(cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		inst.Stop()
		t.Fatal("Start succeeded on a port that is in use")
	}
	if got := nebulactl.ReasonOf(err); got != nebulactl.ReasonPortInUse {
		t.Fatalf("reason = %q, want port_in_use (err: %v)", got, err)
	}
}

// Without privileges, creating the tun device fails. Skipped as root (it
// would succeed) and on Windows (Wintun's error differs; covered by the
// manual tests).
func TestStartTunUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unprivileged Unix user")
	}
	cfg := newPKI(t).ConfigWith(nebulatest.ConfigOptions{Tun: true})
	inst, err := nebulactl.Start(cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		inst.Stop()
		t.Skip("tun device was created; this user has the privileges")
	}
	if got := nebulactl.ReasonOf(err); got != nebulactl.ReasonTunUnavailable {
		t.Fatalf("reason = %q, want tun_unavailable (err: %v)", got, err)
	}
}

func TestErrorUnwraps(t *testing.T) {
	base := errors.New("boom")
	err := error(&nebulactl.Error{Reason: nebulactl.ReasonPortInUse, Err: base})
	if !errors.Is(err, base) || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
	if nebulactl.ReasonOf(base) != "" {
		t.Fatal("ReasonOf an unclassified error is not empty")
	}
}
