// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	good := testConfig()
	if err := validateConfig(good); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name, yaml, want string
	}{
		{"not yaml", "pki: [", "not valid YAML"},
		{"empty", "", "empty"},
		{"not a mapping", "- a\n- b\n", "not valid YAML"},
		{"no pki", "listen:\n  port: 4242\n", "pki section is missing"},
		{"missing key", strings.Replace(good, "  key: |", "  other: |", 1), "pki.key is missing"},
		{"key is a path", strings.Replace(good, "  key: |\n"+indentPEM(testPEM("TEST KEY")), "  key: /etc/shadow\n", 1), "pki.key must be inline PEM"},
		{"ca is a path", strings.Replace(good, "  ca: |\n"+indentPEM(testPEM("TEST CA")), "  ca: ca.crt\n", 1), "pki.ca must be inline PEM"},
		{"cert is a list", strings.Replace(good, "  cert: |\n"+indentPEM(testPEM("TEST CERT")), "  cert: [a]\n", 1), "pki.cert must be a PEM string"},
		{"path with PEM marker later", strings.Replace(good, "  key: |\n", "  key: |\n    /etc/shadow\n", 1), "pki.key must be inline PEM"},
		{"sshd enabled", good + "sshd:\n  enabled: true\n", "sshd.enabled"},
		{"sshd enabled as string", good + "sshd:\n  enabled: \"yes\"\n", "sshd.enabled"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConfig(tc.yaml)
			var ic *errInvalidConfig
			if !errors.As(err, &ic) {
				t.Fatalf("err = %v, want errInvalidConfig", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want it to contain %q", err, tc.want)
			}
		})
	}

	if err := validateConfig(good + "sshd:\n  enabled: false\n"); err != nil {
		t.Fatalf("sshd.enabled: false rejected: %v", err)
	}
}

func TestValidateConfigErrorsDontLeakValues(t *testing.T) {
	bad := strings.Replace(testConfig(), "  key: |\n"+indentPEM(testPEM("TEST KEY")), "  key: /root/secret-file\n", 1)
	if err := validateConfig(bad); err == nil || strings.Contains(err.Error(), "secret-file") {
		t.Fatalf("err = %v", err)
	}
}

func TestRedactConfig(t *testing.T) {
	out, err := redactConfig(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "TEST KEY") || !strings.Contains(out, redacted) {
		t.Fatalf("key not redacted:\n%s", out)
	}
	for _, keep := range []string{"TEST CA", "TEST CERT", "203.0.113.1:4242"} {
		if !strings.Contains(out, keep) {
			t.Errorf("redacted config lost %q:\n%s", keep, out)
		}
	}
}

// A private key reachable another way, here through a YAML anchor, is
// redacted too.
func TestRedactConfigAnchor(t *testing.T) {
	pk := "PRIVATE" + " KEY"
	in := "secret: &k \"" + pk + " material\"\n" +
		"pki:\n  ca: x\n  cert: y\n  key: *k\n"
	out, err := redactConfig(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "material") {
		t.Fatalf("key material survived:\n%s", out)
	}
}
