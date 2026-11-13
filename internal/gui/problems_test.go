// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// catalogue builds every entry, with every platform variant, so the
// content tests cover all of them. A new constructor must be added here;
// TestCatalogueIDs fails otherwise.
func catalogue() []*problem {
	day := time.Date(2027, 3, 3, 12, 0, 0, 0, time.Local)
	var out []*problem
	for _, goos := range []string{"linux", "darwin", "windows"} {
		out = append(out, noPermission(goos), tunUnavailable(goos))
	}
	return append(out,
		serviceNotRunning(), serviceNotResponding(),
		serviceTooOld(0, 1), appTooOld(2, 1),
		noConfig(), notAConfig("notes.txt"),
		fileMissing("key", "laptop.key"), fileUnreadable("key", "laptop.key"),
		fileNotPEM("key", "laptop.key"), fileNotPEM("ca", "ca.crt"),
		pkiMissing(), sshdEnabled(),
		certExpired(&day), certExpired(nil), caExpired(&day), caExpired(nil),
		keyMismatch("laptop.key", "laptop.crt"), keyMismatch("", ""),
		wrongCA(), keyEncrypted("laptop.key"), keyEncrypted(""),
		configTooLarge(), configProblem(),
		portInUse(), nebulaStopped(), newConfigSaved(), connectFailed(),
	)
}

// allIDs lists every catalogue entry.
var allIDs = []problemID{
	probServiceNotRunning, probNoPermission, probServiceNotResponding, probServiceTooOld, probAppTooOld,
	probNoConfig, probNotAConfig, probFileMissing, probFileUnreadable, probFileNotPEM, probPKIMissing,
	probSSHDEnabled, probCertExpired, probCAExpired, probKeyMismatch, probWrongCA, probKeyEncrypted,
	probConfigTooLarge, probConfigProblem,
	probTunUnavailable, probPortInUse, probNebulaStopped, probNewConfigSaved, probConnectFailed,
}

func TestCatalogueIDs(t *testing.T) {
	seen := map[problemID]bool{}
	for _, p := range catalogue() {
		seen[p.ID] = true
	}
	for _, id := range allIDs {
		if !seen[id] {
			t.Errorf("catalogue() has no %s entry", id)
		}
	}
	if len(seen) != len(allIDs) {
		t.Errorf("catalogue() has %d IDs, allIDs %d", len(seen), len(allIDs))
	}
}

func TestCatalogueComplete(t *testing.T) {
	for _, p := range catalogue() {
		if p.ID == "" || p.Title == "" || p.Explain == "" || len(p.Steps) == 0 {
			t.Errorf("%s: incomplete: %+v", p.ID, p)
		}
		for _, s := range p.Steps {
			if s == "" || !strings.HasSuffix(s, ".") {
				t.Errorf("%s: step %q isn't a sentence", p.ID, s)
			}
		}
		if p.Fix != nil && (p.Fix.Label == "" || p.Fix.Kind == 0) {
			t.Errorf("%s: incomplete fix %+v", p.ID, p.Fix)
		}
		if p.Fix != nil && p.Fix.Kind == fixChooseFile && p.Fix.PKIKey == "" {
			t.Errorf("%s: Choose the file without a pki key", p.ID)
		}
	}
}

// jargon is the banned list from "Rules for messages" in docs/m2-plan.md.
var jargon = regexp.MustCompile(`(?i)\b(socket|sockets|pipe|pipes|pem|yaml|tun|api|ipc|daemon|http|https)\b`)

func TestCatalogueNoJargon(t *testing.T) {
	for _, p := range catalogue() {
		for _, s := range append([]string{p.Title, p.Explain}, p.Steps...) {
			if m := jargon.FindString(s); m != "" {
				t.Errorf("%s: %q contains %q", p.ID, s, m)
			}
		}
		if p.Fix != nil {
			if m := jargon.FindString(p.Fix.Label); m != "" {
				t.Errorf("%s: fix label %q contains %q", p.ID, p.Fix.Label, m)
			}
		}
	}
}

func TestJargonPattern(t *testing.T) {
	for s, want := range map[string]bool{
		"the socket":           true,
		"Named Pipe":           true,
		"a YAML file":          true,
		"the tun device":       true,
		"Wintun driver":        false,
		"Turn it off":          false,
		"rapid":                false,
		"the API version":      true,
		"no http here, really": true,
	} {
		if got := jargon.MatchString(s); got != want {
			t.Errorf("jargon.MatchString(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestCatalogueNames(t *testing.T) {
	day := time.Date(2027, 3, 3, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		p    *problem
		want []string
	}{
		{fileMissing("key", "laptop.key"), []string{"laptop.key"}},
		{notAConfig("notes.txt"), []string{"notes.txt"}},
		{certExpired(&day), []string{"3 March 2027"}},
		{caExpired(&day), []string{"3 March 2027"}},
		{keyMismatch("laptop.key", "laptop.crt"), []string{"laptop.key", "laptop.crt"}},
		{fileNotPEM("key", "laptop.key"), []string{"isn't a key file"}},
		{fileNotPEM("cert", "laptop.crt"), []string{"isn't a certificate file"}},
	} {
		all := strings.Join(append([]string{tc.p.Title, tc.p.Explain}, tc.p.Steps...), "\n")
		for _, w := range tc.want {
			if !strings.Contains(all, w) {
				t.Errorf("%s: %q doesn't mention %q", tc.p.ID, all, w)
			}
		}
	}
	if p := noPermission("linux"); !strings.Contains(p.Command, "usermod") {
		t.Errorf("linux command = %q", p.Command)
	}
	if p := noPermission("darwin"); !strings.Contains(p.Command, "dseditgroup") {
		t.Errorf("darwin command = %q", p.Command)
	}
	if p := noPermission("windows"); p.Command != "" || !strings.Contains(p.Steps[0], "Reinstall") {
		t.Errorf("windows = %+v", p)
	}
	if p := tunUnavailable("windows"); !strings.Contains(p.Explain, "Wintun") {
		t.Errorf("windows tun = %q", p.Explain)
	}
}

func TestProblemForAPIError(t *testing.T) {
	notAfter := time.Date(2027, 3, 3, 12, 0, 0, 0, time.Local)
	cert := &api.CertInfo{NotAfter: notAfter, CANotAfter: &notAfter}
	for _, tc := range []struct {
		code   api.ErrorCode
		reason api.Reason
		msg    string
		op     op
		want   problemID
	}{
		{api.CodeInvalidConfig, api.ReasonYAMLSyntax, "", opImport, probNotAConfig},
		{api.CodeInvalidConfig, api.ReasonPKIMissing, "", opImport, probPKIMissing},
		{api.CodeInvalidConfig, api.ReasonSSHDEnabled, "", opImport, probSSHDEnabled},
		{api.CodeInvalidConfig, api.ReasonCertExpired, "", opImport, probCertExpired},
		{api.CodeInvalidConfig, api.ReasonCertExpired, "", opConnect, probCertExpired},
		{api.CodeInvalidConfig, api.ReasonCAExpired, "", opConnect, probCAExpired},
		{api.CodeInvalidConfig, api.ReasonKeyMismatch, "", opImport, probKeyMismatch},
		{api.CodeInvalidConfig, api.ReasonWrongCA, "", opImport, probWrongCA},
		{api.CodeInvalidConfig, api.ReasonKeyEncrypted, "", opImport, probKeyEncrypted},
		{api.CodeInvalidConfig, "", "", opImport, probConfigProblem},
		{api.CodeInvalidConfig, "something_new", "", opImport, probConfigProblem},
		{api.CodeBadRequest, "", "request body too large", opImport, probConfigTooLarge},
		{api.CodeBadRequest, "", "yaml is required", opImport, probConfigProblem},
		{api.CodeInternal, api.ReasonTunUnavailable, "", opConnect, probTunUnavailable},
		{api.CodeInternal, api.ReasonPortInUse, "", opConnect, probPortInUse},
		{api.CodeInternal, api.ReasonNebulaStopped, "", opConnect, probNebulaStopped},
		{api.CodeInternal, "", "", opConnect, probConnectFailed},
		{api.CodeInternal, "something_new", "", opConnect, probConnectFailed},
		{api.CodeNotConfigured, "", "", opConnect, probNoConfig},
	} {
		e := &api.Error{Code: tc.code, Reason: tc.reason, Message: tc.msg}
		// The client wraps nothing around *api.Error today, but a caller may.
		p := problemFor(fmt.Errorf("connect: %w", e), problemContext{Op: tc.op, Cert: cert, ConfigName: "notes.txt"})
		if p.ID != tc.want {
			t.Errorf("%s/%s op %d = %s, want %s", tc.code, tc.reason, tc.op, p.ID, tc.want)
		}
		if !strings.Contains(p.Details, string(tc.code)) {
			t.Errorf("%s/%s: details %q lack the code", tc.code, tc.reason, p.Details)
		}
	}

	p := problemFor(&api.Error{Code: api.CodeInvalidConfig, Reason: api.ReasonCertExpired, Message: "expired"},
		problemContext{Cert: cert})
	if !strings.Contains(p.Explain, "3 March 2027") || p.Details != "invalid_config / cert_expired: expired" {
		t.Errorf("cert_expired = %+v", p)
	}
}

func TestProblemForTimeout(t *testing.T) {
	for _, err := range []error{
		context.DeadlineExceeded,
		fmt.Errorf("GET /v1/status: %w", &url.Error{Op: "Get", URL: "http://nebula-guid/v1/status", Err: context.DeadlineExceeded}),
		&url.Error{Op: "Get", URL: "x", Err: timeoutErr{}},
	} {
		if p := problemFor(err, problemContext{}); p.ID != probServiceNotResponding {
			t.Errorf("%v = %s", err, p.ID)
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "Client.Timeout exceeded" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestProblemForBrokenConnection(t *testing.T) {
	for _, err := range []error{io.EOF, fmt.Errorf("GET /v1/status: %w", io.ErrUnexpectedEOF)} {
		if p := problemFor(err, problemContext{}); p.ID != probServiceNotRunning {
			t.Errorf("%v = %s", err, p.ID)
		}
	}
	if p := problemFor(errors.New("decode response: bad"), problemContext{}); p.ID != probServiceNotResponding {
		t.Errorf("unknown error = %s", p.ID)
	}
	if problemFor(nil, problemContext{}) != nil {
		t.Error("problemFor(nil) != nil")
	}
}

func TestStatusProblem(t *testing.T) {
	notAfter := time.Date(2027, 3, 3, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name string
		st   *api.StatusResponse
		want problemID
	}{
		{"nil", nil, ""},
		{"not configured", &api.StatusResponse{State: api.StateStopped}, probNoConfig},
		{"running", &api.StatusResponse{State: api.StateRunning, Configured: true, Cert: &api.CertInfo{NotAfter: notAfter}}, ""},
		{"cert expired", &api.StatusResponse{State: api.StateStopped, Configured: true,
			Cert: &api.CertInfo{NotAfter: notAfter, Expired: true}}, probCertExpired},
		{"ca expired wins", &api.StatusResponse{State: api.StateStopped, Configured: true,
			Cert: &api.CertInfo{Expired: true, CAExpired: true, CANotAfter: &notAfter}}, probCAExpired},
		{"stopped by itself", &api.StatusResponse{State: api.StateError, Configured: true,
			Error: "boom", ErrorReason: api.ReasonNebulaStopped}, probNebulaStopped},
		{"error without reason", &api.StatusResponse{State: api.StateError, Configured: true, Error: "boom"}, probConnectFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := statusProblem(tc.st, "linux")
			var id problemID
			if p != nil {
				id = p.ID
			}
			if id != tc.want {
				t.Fatalf("statusProblem = %q, want %q", id, tc.want)
			}
			if tc.st != nil && tc.st.Error != "" && !strings.Contains(p.Details, tc.st.Error) {
				t.Fatalf("details %q lack the error", p.Details)
			}
		})
	}
}

func TestVersionProblem(t *testing.T) {
	if p := versionProblem(&api.VersionResponse{APIVersion: api.APIVersion}); p != nil {
		t.Fatalf("same version = %+v", p)
	}
	if p := versionProblem(&api.VersionResponse{APIVersion: api.APIVersion + 1}); p.ID != probAppTooOld {
		t.Fatalf("newer service = %s", p.ID)
	}
	if p := versionProblem(&api.VersionResponse{APIVersion: api.APIVersion - 1}); p.ID != probServiceTooOld {
		t.Fatalf("older service = %s", p.ID)
	}
}

func TestNumberSteps(t *testing.T) {
	if got := numberSteps([]string{"One."}); got != "One." {
		t.Errorf("one step = %q", got)
	}
	if got := numberSteps([]string{"One.", "Two."}); got != "1. One.\n2. Two." {
		t.Errorf("two steps = %q", got)
	}
}
