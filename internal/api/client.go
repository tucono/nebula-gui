// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/tucono/nebula-gui/internal/ipc"
)

// baseURL is a placeholder host. Every request is sent over the IPC
// transport, so the host is never resolved.
const baseURL = "http://nebula-guid"

// maxResponseBytes caps how much of a response the client will read.
const maxResponseBytes = 4 << 20

// Client is a typed client for the service API.
type Client struct {
	addr string
	hc   *http.Client
}

// NewClient returns a client that talks to the service at addr, a Unix
// socket path or a Windows named pipe.
func NewClient(addr string) *Client {
	return newClient(addr, func(ctx context.Context) (net.Conn, error) {
		return ipc.Dial(ctx, addr)
	})
}

func newClient(addr string, dial func(ctx context.Context) (net.Conn, error)) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dial(ctx)
		},
		MaxIdleConns:    2,
		IdleConnTimeout: 30 * time.Second,
	}
	return &Client{
		addr: addr,
		hc:   &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}
}

// Addr returns the service address the client connects to.
func (c *Client) Addr() string { return c.addr }

// Version calls GET /v1/version.
func (c *Client) Version(ctx context.Context) (*VersionResponse, error) {
	var v VersionResponse
	if err := c.do(ctx, http.MethodGet, "/v1/version", nil, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Status calls GET /v1/status.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	var s StatusResponse
	if err := c.do(ctx, http.MethodGet, "/v1/status", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// PutConfig calls PUT /v1/config.
func (c *Client) PutConfig(ctx context.Context, yaml string) (*ConfigPutResponse, error) {
	var r ConfigPutResponse
	if err := c.do(ctx, http.MethodPut, "/v1/config", ConfigRequest{YAML: yaml}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Config calls GET /v1/config. The returned YAML has pki.key redacted.
func (c *Client) Config(ctx context.Context) (string, error) {
	var r ConfigResponse
	if err := c.do(ctx, http.MethodGet, "/v1/config", nil, &r); err != nil {
		return "", err
	}
	return r.YAML, nil
}

// DeleteConfig calls DELETE /v1/config, which stops Nebula first if it is
// running.
func (c *Client) DeleteConfig(ctx context.Context) (*StatusResponse, error) {
	return c.statusCall(ctx, http.MethodDelete, "/v1/config")
}

// Connect calls POST /v1/connect.
func (c *Client) Connect(ctx context.Context) (*StatusResponse, error) {
	return c.statusCall(ctx, http.MethodPost, "/v1/connect")
}

// Disconnect calls POST /v1/disconnect.
func (c *Client) Disconnect(ctx context.Context) (*StatusResponse, error) {
	return c.statusCall(ctx, http.MethodPost, "/v1/disconnect")
}

// Logs calls GET /v1/logs and returns lines with seq >= since.
func (c *Client) Logs(ctx context.Context, since uint64) (*LogsResponse, error) {
	var r LogsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/logs?since="+strconv.FormatUint(since, 10), nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Diagnostics calls GET /v1/diagnostics. lines <= 0 asks for the default
// number of log lines.
func (c *Client) Diagnostics(ctx context.Context, lines int) (*DiagnosticsResponse, error) {
	path := "/v1/diagnostics"
	if lines > 0 {
		path += "?lines=" + strconv.Itoa(lines)
	}
	var r DiagnosticsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) statusCall(ctx context.Context, method, path string) (*StatusResponse, error) {
	var s StatusResponse
	if err := c.do(ctx, method, path, nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode %s %s request: %w", method, path, err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build %s %s request: %w", method, path, err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%s %s: read response: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		return decodeError(resp.StatusCode, resp.Status, data)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}

func decodeError(code int, status string, data []byte) error {
	var eb ErrorBody
	if err := json.Unmarshal(data, &eb); err != nil || eb.Error == nil || eb.Error.Code == "" {
		return &Error{Code: CodeInternal, Message: "unexpected response: " + status, HTTPStatus: code}
	}
	eb.Error.HTTPStatus = code
	return eb.Error
}

// ErrorCodeOf returns the service error code carried by err, or "" if err
// did not come from the service.
func ErrorCodeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
