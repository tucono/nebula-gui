// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func fakePEM(typ, body string) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: []byte(body)}))
}

var (
	caPEM   = fakePEM("NEBULA CERTIFICATE V2", "ca")
	ca2PEM  = fakePEM("NEBULA CERTIFICATE V2", "ca2")
	certPEM = fakePEM("NEBULA CERTIFICATE V2", "laptop")
	keyPEM  = fakePEM("NEBULA X25519 PRIVATE KEY", "secret")
)

// fakeFS reads from a map; a missing entry is fs.ErrNotExist.
type fakeFS map[string]string

func (f fakeFS) read(path string) ([]byte, error) {
	s, ok := f[filepath.ToSlash(path)]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	if s == "<denied>" {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrPermission}
	}
	return []byte(s), nil
}

// abs makes a slash path absolute on this OS, so the tests run on Windows.
func abs(p string) string {
	if runtime.GOOS == "windows" {
		return `C:` + filepath.FromSlash(p)
	}
	return p
}

func opts(files fakeFS) inlineOptions {
	f := fakeFS{}
	for k, v := range files {
		f[filepath.ToSlash(abs(k))] = v
	}
	return inlineOptions{BaseDir: abs("/home/alice/Downloads"), Home: abs("/home/alice"), Picked: map[string]string{}, Read: f.read}
}

// pkiOf returns the pki values of a config.
func pkiOf(t *testing.T, s string) map[string]string {
	t.Helper()
	var v struct{ PKI map[string]string }
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("result doesn't parse: %v\n%s", err, s)
	}
	return v.PKI
}

func TestInlineAlreadyInline(t *testing.T) {
	raw := "pki:\n  ca: |\n" + indent(caPEM) + "  cert: |\n" + indent(certPEM) + "  key: |\n" + indent(keyPEM)
	out, err := inlinePKI([]byte(raw), opts(nil))
	if err != nil {
		t.Fatal(err)
	}
	if p := pkiOf(t, out.YAML); p["ca"] != caPEM || p["cert"] != certPEM || p["key"] != keyPEM {
		t.Fatalf("changed: %v", p)
	}
	if len(out.Names) != 0 {
		t.Fatalf("names %v", out.Names)
	}
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(strings.TrimSuffix(s, "\n"), "\n", "\n    ") + "\n"
}

func TestInlinePaths(t *testing.T) {
	raw := `# from the admin
pki:
  ca: /etc/nebula/ca.crt   # absolute
  cert: laptop.crt
  key: ~/keys/laptop.key
lighthouse:
  hosts: [192.168.100.1]
`
	o := opts(fakeFS{
		"/etc/nebula/ca.crt":               caPEM + ca2PEM, // a bundle
		"/home/alice/Downloads/laptop.crt": certPEM,
		"/home/alice/keys/laptop.key":      keyPEM,
	})
	out, err := inlinePKI([]byte(raw), o)
	if err != nil {
		t.Fatal(err)
	}
	p := pkiOf(t, out.YAML)
	if p["ca"] != caPEM+ca2PEM || p["cert"] != certPEM || p["key"] != keyPEM {
		t.Fatalf("pki %v", p)
	}
	want := map[string]string{"ca": "ca.crt", "cert": "laptop.crt", "key": "laptop.key"}
	if !reflect.DeepEqual(out.Names, want) {
		t.Fatalf("names %v", out.Names)
	}
	for _, s := range []string{"# from the admin", "lighthouse:", "ca: |"} {
		if !strings.Contains(out.YAML, s) {
			t.Errorf("result lacks %q:\n%s", s, out.YAML)
		}
	}
	if strings.Index(out.YAML, "pki:") > strings.Index(out.YAML, "lighthouse:") {
		t.Error("key order changed")
	}
}

func TestInlineMissingThenPicked(t *testing.T) {
	raw := "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: /etc/nebula/laptop.key\n"
	o := opts(fakeFS{
		"/home/alice/Downloads/ca.crt":     caPEM,
		"/home/alice/Downloads/laptop.crt": certPEM,
		"/home/alice/Desktop/my.key":       keyPEM,
	})
	_, err := inlinePKI([]byte(raw), o)
	var missing *errFileMissing
	if !errors.As(err, &missing) || missing.Key != "key" || baseName(missing.Path) != "laptop.key" {
		t.Fatalf("got %v", err)
	}
	if p := inlineProblem(err, "config.yml"); p.ID != probFileMissing || p.Title != "A file is missing: laptop.key" ||
		p.Fix.Kind != fixChooseFile || p.Fix.PKIKey != "key" {
		t.Fatalf("problem %+v", p)
	}

	o.Picked["key"] = abs("/home/alice/Desktop/my.key")
	out, err := inlinePKI([]byte(raw), o)
	if err != nil {
		t.Fatal(err)
	}
	if pkiOf(t, out.YAML)["key"] != keyPEM || out.Names["key"] != "my.key" {
		t.Fatalf("picked file not used: %v", out.Names)
	}
}

func TestInlineErrors(t *testing.T) {
	files := fakeFS{
		"/home/alice/Downloads/ca.crt":     caPEM,
		"/home/alice/Downloads/laptop.crt": certPEM,
		"/home/alice/Downloads/laptop.key": keyPEM,
		"/home/alice/Downloads/notes.txt":  "just some text\n",
		"/home/alice/Downloads/denied.key": "<denied>",
		"/home/alice/Downloads/big.key":    strings.Repeat("x", maxFileBytes+1),
	}
	cases := []struct {
		name, raw string
		want      problemID
	}{
		{"not yaml", "pki: [unclosed\n", probNotAConfig},
		{"empty", "", probNotAConfig},
		{"txt", "Remember to buy milk.\n", probNotAConfig},
		{"list", "- pki\n- ca\n", probNotAConfig},
		{"no pki", "lighthouse:\n  hosts: []\n", probNotAConfig},
		{"pki not a map", "pki: yes\n", probPKIMissing},
		{"no key", "pki:\n  ca: ca.crt\n  cert: laptop.crt\n", probPKIMissing},
		{"empty key", "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: ''\n", probPKIMissing},
		{"not pem", "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: notes.txt\n", probFileNotPEM},
		{"cert as key", "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.crt\n", probFileNotPEM},
		{"key as cert", "pki:\n  ca: ca.crt\n  cert: laptop.key\n  key: laptop.key\n", probFileNotPEM},
		{"unreadable", "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: denied.key\n", probFileUnreadable},
		{"huge file", "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: big.key\n", probFileNotPEM},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := inlinePKI([]byte(c.raw), opts(files))
			if err == nil {
				t.Fatal("no error")
			}
			p := inlineProblem(err, "notes.txt")
			if p == nil || p.ID != c.want {
				t.Fatalf("%v: problem %+v, want %s", err, p, c.want)
			}
			if !strings.Contains(p.Details, err.Error()) {
				t.Errorf("details %q lack the error", p.Details)
			}
		})
	}
}

func TestInlineResultTooLarge(t *testing.T) {
	// Each file is under the per-file cap, but together they aren't.
	big := fakePEM("NEBULA CERTIFICATE V2", strings.Repeat("c", 500_000))
	raw := "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.key\n"
	_, err := inlinePKI([]byte(raw), opts(fakeFS{
		"/home/alice/Downloads/ca.crt":     big,
		"/home/alice/Downloads/laptop.crt": big,
		"/home/alice/Downloads/laptop.key": keyPEM,
	}))
	var tl *errTooLarge
	if !errors.As(err, &tl) || tl.Key != "" {
		t.Fatalf("got %v", err)
	}
	if p := inlineProblem(err, "config.yml"); p.ID != probConfigTooLarge {
		t.Fatalf("problem %s", p.ID)
	}
}

func TestInlineDisableSSHD(t *testing.T) {
	raw := `pki:
  ca: ca.crt
  cert: laptop.crt
  key: laptop.key
sshd:
  enabled: true # debug console
  listen: 127.0.0.1:2222
  authorized_users:
    - user: admin
      keys: []
tun:
  dev: nebula1
`
	o := opts(fakeFS{
		"/home/alice/Downloads/ca.crt":     caPEM,
		"/home/alice/Downloads/laptop.crt": certPEM,
		"/home/alice/Downloads/laptop.key": keyPEM,
	})
	before, err := inlinePKI([]byte(raw), o)
	if err != nil {
		t.Fatal(err)
	}
	o.DisableSSHD = true
	after, err := inlinePKI([]byte(raw), o)
	if err != nil {
		t.Fatal(err)
	}

	var b, a map[string]any
	if err := yaml.Unmarshal([]byte(before.YAML), &b); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(after.YAML), &a); err != nil {
		t.Fatal(err)
	}
	if a["sshd"].(map[string]any)["enabled"] != false {
		t.Fatalf("sshd.enabled is %v", a["sshd"])
	}
	b["sshd"].(map[string]any)["enabled"] = false
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("more than sshd.enabled changed:\n%s\n---\n%s", before.YAML, after.YAML)
	}
	if !strings.Contains(after.YAML, "# debug console") {
		t.Error("comment lost")
	}
}

func TestInlineCRLF(t *testing.T) {
	raw := "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.key\n"
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	out, err := inlinePKI([]byte(raw), opts(fakeFS{
		"/home/alice/Downloads/ca.crt":     crlf(caPEM),
		"/home/alice/Downloads/laptop.crt": crlf(certPEM),
		"/home/alice/Downloads/laptop.key": crlf(keyPEM),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if pkiOf(t, out.YAML)["key"] != keyPEM || !strings.Contains(out.YAML, "key: |") {
		t.Fatalf("CRLF not normalised:\n%s", out.YAML)
	}
}

func TestResolvePath(t *testing.T) {
	base, home := abs("/cfg"), abs("/home/alice")
	cases := map[string]string{
		"ca.crt":           abs("/cfg/ca.crt"),
		"keys/host.key":    abs("/cfg/keys/host.key"),
		"../ca.crt":        abs("/ca.crt"),
		"~/host.key":       abs("/home/alice/host.key"),
		abs("/etc/ca.crt"): abs("/etc/ca.crt"),
	}
	for in, want := range cases {
		if got := resolvePath(in, base, home); got != want {
			t.Errorf("resolvePath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := baseName(`C:\nebula\laptop.key`); got != "laptop.key" {
		t.Errorf("baseName = %q", got)
	}
}

// TestInlineRealFiles reads real files, with relative paths and readCapped.
func TestInlineRealFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("ca.crt", caPEM)
	write("keys/laptop.crt", certPEM)
	write("keys/laptop.key", keyPEM)
	raw := "pki:\n  ca: ca.crt\n  cert: keys/laptop.crt\n  key: ./keys/laptop.key\n"
	out, err := inlinePKI([]byte(raw), inlineOptions{BaseDir: dir, Read: readCapped})
	if err != nil {
		t.Fatal(err)
	}
	if p := pkiOf(t, out.YAML); p["key"] != keyPEM || p["ca"] != caPEM {
		t.Fatalf("pki %v", p)
	}

	_, err = inlinePKI([]byte("pki:\n  ca: gone.crt\n  cert: x\n  key: y\n"), inlineOptions{BaseDir: dir, Read: readCapped})
	var missing *errFileMissing
	if !errors.As(err, &missing) || missing.Path != filepath.Join(dir, "gone.crt") {
		t.Fatalf("got %v", err)
	}
}

func TestLogErrorHasNoPaths(t *testing.T) {
	errs := []error{
		&errFileMissing{Key: "key", Path: abs("/etc/nebula/laptop.key")},
		&errFileUnreadable{Key: "key", Path: abs("/home/alice/laptop.key"), Err: &fs.PathError{Op: "open", Path: abs("/home/alice/laptop.key"), Err: fs.ErrPermission}},
		&errNotPEM{Key: "cert", Path: abs("/home/alice/laptop.crt")},
		&errTooLarge{Key: "ca", Path: abs("/home/alice/ca.crt")},
	}
	for _, err := range errs {
		s := logError(err)
		if strings.Contains(s, "alice") || strings.Contains(s, "nebula/") || strings.Contains(s, `\`) {
			t.Errorf("logError(%v) = %q has a path", err, s)
		}
	}
}
