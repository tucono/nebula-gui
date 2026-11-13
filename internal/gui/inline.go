// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	// maxFileBytes caps the config file and each file it refers to.
	maxFileBytes = 1 << 20
	// maxBodyBytes is the service's limit on a request body (maxBodyBytes
	// in internal/service). The inlined config is checked against it, as
	// JSON, before it's sent.
	maxBodyBytes = 1 << 20
)

// pkiKeys are the pki entries that name a file or hold its PEM.
var pkiKeys = []string{"ca", "cert", "key"}

// inlineOptions are the inputs to inlinePKI besides the config itself.
type inlineOptions struct {
	// BaseDir is the config file's directory. Relative paths resolve
	// against it.
	BaseDir string
	// Home replaces a leading "~/" in a path.
	Home string
	// Picked holds files the user chose for a pki entry ("ca", "cert" or
	// "key"); they replace the path the config gives.
	Picked map[string]string
	// DisableSSHD sets sshd.enabled to false.
	DisableSSHD bool
	// Read reads a file. Tests pass a fake; the app passes readCapped.
	Read func(path string) ([]byte, error)
}

// inlined is a config ready for PUT /v1/config.
type inlined struct {
	YAML string
	// Names are the file names of the pki entries that were paths, by
	// key, for messages such as "laptop.key doesn't match laptop.crt".
	Names map[string]string
}

// The typed errors below let the import dialog pick the catalogue entry
// and the file a Choose the file… button is for.

// errNotConfig: the file isn't YAML, isn't a mapping, or has no pki key.
type errNotConfig struct{ Why string }

// errPKIIncomplete: pki is there but an entry is missing or not text.
type errPKIIncomplete struct{ Key string }

// errFileMissing: a pki path doesn't exist on this computer.
type errFileMissing struct{ Key, Path string }

// errFileUnreadable: a pki path exists but can't be read.
type errFileUnreadable struct {
	Key, Path string
	Err       error
}

// errNotPEM: a pki file doesn't hold the PEM its entry needs.
type errNotPEM struct{ Key, Path string }

// errTooLarge: a pki file (Key set) or the finished config (Key empty) is
// over its limit.
type errTooLarge struct{ Key, Path string }

func (e *errNotConfig) Error() string { return "not a nebula config: " + e.Why }
func (e *errPKIIncomplete) Error() string {
	return fmt.Sprintf("pki.%s is missing or not a string", e.Key)
}
func (e *errFileMissing) Error() string {
	return fmt.Sprintf("pki.%s: %s does not exist", e.Key, e.Path)
}
func (e *errFileUnreadable) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("can't read %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("pki.%s: can't read %s: %v", e.Key, e.Path, e.Err)
}
func (e *errFileUnreadable) Unwrap() error { return e.Err }
func (e *errNotPEM) Error() string {
	return fmt.Sprintf("pki.%s: %s is not a PEM %s", e.Key, e.Path, pemWant(e.Key))
}
func (e *errTooLarge) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("config is larger than %d bytes once the pki files are inlined", maxBodyBytes)
	}
	return fmt.Sprintf("pki.%s: %s is larger than %d bytes", e.Key, e.Path, maxFileBytes)
}

// inlinePKI replaces the pki.ca, pki.cert and pki.key paths in a config
// with the PEM they point to, because the service only takes inline PEM.
// It edits the YAML tree, so comments and key order are kept.
//
// A value that already is PEM stays as is. Anything else is a path, used as
// written: relative to the config's directory, with ~/ expanded. There is
// no guessing: a path that doesn't exist is an errFileMissing, and the
// user picks the file (opts.Picked).
func inlinePKI(raw []byte, opts inlineOptions) (*inlined, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, &errNotConfig{Why: err.Error()}
	}
	root := docRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, &errNotConfig{Why: "the top level is not a mapping"}
	}
	pki := resolve(mapValue(root, "pki"))
	if pki == nil {
		return nil, &errNotConfig{Why: "no pki section"}
	}
	if pki.Kind != yaml.MappingNode {
		return nil, &errPKIIncomplete{Key: "ca"}
	}

	out := &inlined{Names: map[string]string{}}
	for _, k := range pkiKeys {
		n := resolve(mapValue(pki, k))
		if n == nil || n.Kind != yaml.ScalarNode || strings.TrimSpace(n.Value) == "" {
			return nil, &errPKIIncomplete{Key: k}
		}
		path := opts.Picked[k]
		if path == "" {
			if strings.HasPrefix(strings.TrimSpace(n.Value), "-----BEGIN") {
				continue
			}
			path = resolvePath(strings.TrimSpace(n.Value), opts.BaseDir, opts.Home)
			out.Names[k] = baseName(n.Value)
		} else {
			out.Names[k] = filepath.Base(path)
		}

		data, err := opts.Read(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil, &errFileMissing{Key: k, Path: path}
		case err != nil:
			return nil, &errFileUnreadable{Key: k, Path: path, Err: err}
		case len(data) > maxFileBytes:
			return nil, &errTooLarge{Key: k, Path: path}
		}
		pemText, ok := checkPEM(k, data)
		if !ok {
			return nil, &errNotPEM{Key: k, Path: path}
		}
		n.Kind, n.Tag, n.Style, n.Value = yaml.ScalarNode, "!!str", yaml.LiteralStyle, pemText
	}

	if opts.DisableSSHD {
		if sshd := resolve(mapValue(root, "sshd")); sshd != nil && sshd.Kind == yaml.MappingNode {
			if en := resolve(mapValue(sshd, "enabled")); en != nil {
				en.Kind, en.Tag, en.Style, en.Value = yaml.ScalarNode, "!!bool", 0, "false"
				en.Content = nil
			}
		}
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	out.YAML = buf.String()

	body, err := json.Marshal(api.ConfigRequest{YAML: out.YAML})
	if err != nil {
		return nil, err
	}
	if len(body) > maxBodyBytes {
		return nil, &errTooLarge{}
	}
	return out, nil
}

// resolvePath turns a path from the config into one on this computer.
func resolvePath(p, baseDir, home string) string {
	switch {
	case p == "~" && home != "":
		return home
	case (strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`)) && home != "":
		return filepath.Join(home, p[2:])
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	default:
		return filepath.Join(baseDir, p)
	}
}

// baseName is the file name of a path written for any OS: the admin's
// config may use / or \.
func baseName(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// checkPEM reports whether data is only PEM blocks of the kind key needs
// (a private key for "key", certificates otherwise), and returns it with
// line endings normalised so it can be a literal block.
func checkPEM(key string, data []byte) (string, bool) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest := []byte(text)
	blocks := 0
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if !strings.Contains(b.Type, pemWant(key)) {
			return "", false
		}
		blocks++
	}
	if blocks == 0 || len(bytes.TrimSpace(rest)) > 0 {
		return "", false
	}
	text = strings.TrimSpace(text) + "\n"
	return text, true
}

func pemWant(key string) string {
	if key == "key" {
		return "PRIVATE KEY"
	}
	return "CERTIFICATE"
}

// readCapped reads at most maxFileBytes+1 bytes of path, so a huge file is
// found too large without being read whole.
func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the file the config names, or the user chose
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, maxFileBytes+1))
}

// docRoot returns the top node of a parsed document, or nil.
func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		return doc.Content[0]
	}
	return nil
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// resolve follows an alias to the node it names.
func resolve(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}
