// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tucono/nebula-gui/internal/api"
)

// redacted replaces secrets in GET /v1/config.
const redacted = "<redacted>"

// errInvalidConfig marks config validation failures. Its message is safe to
// return to the client: it never includes config values.
type errInvalidConfig struct {
	msg    string
	reason api.Reason // optional
	err    error      // the underlying error, if any
}

func (e *errInvalidConfig) Error() string { return e.msg }
func (e *errInvalidConfig) Unwrap() error { return e.err }

func invalidf(reason api.Reason, format string, args ...any) error {
	return &errInvalidConfig{msg: fmt.Sprintf(format, args...), reason: reason}
}

// validateConfig checks the rules the service enforces before Nebula sees a
// config (docs/security.md):
//   - pki.ca, pki.cert and pki.key are inline PEM, never file paths, so the
//     service can't be used to read files as root;
//   - the sshd debug console is off.
//
// Nebula's own dry run (Engine.Check) runs afterwards.
func validateConfig(raw string) error {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		return invalidf(api.ReasonYAMLSyntax, "config is not valid YAML: %s", yamlErrMsg(err))
	}
	if root == nil {
		return invalidf("", "config is empty")
	}

	pki, ok := root["pki"].(map[string]any)
	if !ok {
		return invalidf(api.ReasonPKIMissing, "pki section is missing")
	}
	for _, k := range []string{"ca", "cert", "key"} {
		v, ok := pki[k]
		if !ok {
			return invalidf(api.ReasonPKIMissing, "pki.%s is missing", k)
		}
		s, ok := v.(string)
		if !ok {
			return invalidf("", "pki.%s must be a PEM string", k)
		}
		if !strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN ") {
			return invalidf("", "pki.%s must be inline PEM, not a file path", k)
		}
	}

	if sshd, ok := root["sshd"].(map[string]any); ok {
		if v, set := sshd["enabled"]; set && v != false {
			return invalidf(api.ReasonSSHDEnabled, "sshd.enabled must be false: the debug console is not allowed")
		}
	}
	return nil
}

// yamlErrMsg strips the "yaml: " prefix. YAML parse errors give line
// numbers, not values.
func yamlErrMsg(err error) string {
	var te *yaml.TypeError
	if errors.As(err, &te) {
		return "unexpected structure"
	}
	return strings.TrimPrefix(err.Error(), "yaml: ")
}

// redactConfig returns raw with pki.key, and any other value holding a
// private key (for example through a YAML anchor), replaced. Comments and
// key order are kept.
func redactConfig(raw string) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("parse stored config: %w", err)
	}
	if v := mapValue(mapValue(docRoot(&doc), "pki"), "key"); v != nil {
		redactNode(v)
	}
	walk(&doc, func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && strings.Contains(n.Value, "PRIVATE KEY") {
			redactNode(n)
		}
	})

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("encode redacted config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("encode redacted config: %w", err)
	}
	return buf.String(), nil
}

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

func redactNode(n *yaml.Node) {
	*n = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: redacted, Anchor: n.Anchor}
}

func walk(n *yaml.Node, fn func(*yaml.Node)) {
	fn(n)
	for _, c := range n.Content {
		walk(c, fn)
	}
}
