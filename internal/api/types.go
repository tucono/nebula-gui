// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package api defines the GUI ↔ service contract: request and response
// types, error codes, and a typed client. Both binaries import it, so the
// contract lives in one place. See docs/ipc-api.md.
package api

import (
	"fmt"
	"time"
)

// APIVersion is the version of the API this build speaks. Breaking changes
// bump it together with the path prefix (/v1 → /v2).
const APIVersion = 1

// VersionResponse is returned by GET /v1/version.
type VersionResponse struct {
	ServiceVersion string `json:"service_version"`
	NebulaVersion  string `json:"nebula_version"`
	APIVersion     int    `json:"api_version"`
}

// State is the Nebula manager's state.
type State string

// Nebula manager states.
const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateError    State = "error"
)

// CertInfo describes the host certificate of the active config.
type CertInfo struct {
	Name        string    `json:"name"`
	Groups      []string  `json:"groups"`
	NotAfter    time.Time `json:"not_after"`
	Fingerprint string    `json:"fingerprint"`
	// Expired is computed on every status call.
	Expired bool `json:"expired"`
	// CANotAfter is when the CA that signed the cert expires; nil if that
	// CA isn't in pki.ca.
	CANotAfter *time.Time `json:"ca_not_after,omitempty"`
	// CAExpired is computed on every status call.
	CAExpired bool `json:"ca_expired"`
}

// StatusResponse is returned by GET /v1/status.
type StatusResponse struct {
	State State  `json:"state"`
	Error string `json:"error"`
	// ErrorReason says why the state is error, when the service knows.
	ErrorReason Reason     `json:"error_reason,omitempty"`
	OverlayIPs  []string   `json:"overlay_ips"`
	Cert        *CertInfo  `json:"cert"`
	ListenPort  int        `json:"listen_port"`
	StartedAt   *time.Time `json:"started_at"`
	Configured  bool       `json:"configured"`
}

// ErrorCode is a machine-readable error code.
type ErrorCode string

// Error codes. See docs/ipc-api.md.
const (
	CodeBadRequest     ErrorCode = "bad_request"
	CodeInvalidConfig  ErrorCode = "invalid_config"
	CodeNotConfigured  ErrorCode = "not_configured"
	CodeAlreadyRunning ErrorCode = "already_running"
	CodeNotRunning     ErrorCode = "not_running"
	CodePingFailed     ErrorCode = "ping_failed"
	CodeInternal       ErrorCode = "internal"
)

// Reason refines an error code: it says why something failed, so clients
// can show a specific message without parsing English. Clients must treat
// an unknown or missing reason like no reason. See docs/ipc-api.md.
type Reason string

// Reasons for invalid_config.
const (
	ReasonYAMLSyntax   Reason = "yaml_syntax"
	ReasonPKIMissing   Reason = "pki_missing"
	ReasonSSHDEnabled  Reason = "sshd_enabled"
	ReasonCertExpired  Reason = "cert_expired"
	ReasonCAExpired    Reason = "ca_expired"
	ReasonKeyMismatch  Reason = "key_mismatch"
	ReasonWrongCA      Reason = "wrong_ca"
	ReasonKeyEncrypted Reason = "key_encrypted"
)

// Reasons for internal errors from connect, and for state=error.
const (
	ReasonTunUnavailable Reason = "tun_unavailable"
	ReasonPortInUse      Reason = "port_in_use"
	ReasonNebulaStopped  Reason = "nebula_stopped"
)

// ErrorBody is the JSON envelope for every error response.
type ErrorBody struct {
	Error *Error `json:"error"`
}

// Error is an error returned by the service.
type Error struct {
	Code ErrorCode `json:"code"`
	// Reason is optional; see Reason.
	Reason  Reason `json:"reason,omitempty"`
	Message string `json:"message"`

	// HTTPStatus is the response status code. It is set by the client and
	// is not part of the JSON body.
	HTTPStatus int `json:"-"`
}

func (e *Error) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Reason, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ConfigRequest is the body of PUT /v1/config.
type ConfigRequest struct {
	// YAML is a full Nebula config with pki.ca, pki.cert and pki.key
	// inlined as PEM.
	YAML string `json:"yaml"`
}

// ConfigPutResponse is returned by PUT /v1/config.
type ConfigPutResponse struct {
	// RestartRequired is set when Nebula is running with the previous
	// config. The service does not restart on its own.
	RestartRequired bool `json:"restart_required"`
}

// ConfigResponse is returned by GET /v1/config. pki.key is redacted.
type ConfigResponse struct {
	YAML string `json:"yaml"`
}

// LogsResponse is returned by GET /v1/logs.
type LogsResponse struct {
	// Next is the seq to pass as ?since= to get only newer lines.
	Next  uint64    `json:"next"`
	Lines []LogLine `json:"lines"`
}

// LogLine is one entry from the service's log ring buffer.
type LogLine struct {
	Seq    uint64         `json:"seq"`
	Time   time.Time      `json:"time"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"fields,omitempty"`
}
