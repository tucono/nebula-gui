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
}

// StatusResponse is returned by GET /v1/status.
type StatusResponse struct {
	State      State      `json:"state"`
	Error      string     `json:"error"`
	OverlayIPs []string   `json:"overlay_ips"`
	Cert       *CertInfo  `json:"cert"`
	ListenPort int        `json:"listen_port"`
	StartedAt  *time.Time `json:"started_at"`
	Configured bool       `json:"configured"`
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

// ErrorBody is the JSON envelope for every error response.
type ErrorBody struct {
	Error *Error `json:"error"`
}

// Error is an error returned by the service.
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`

	// HTTPStatus is the response status code. It is set by the client and
	// is not part of the JSON body.
	HTTPStatus int `json:"-"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
