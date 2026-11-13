// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package service implements the privileged service: the HTTP API served
// over the IPC transport, the config store, the Nebula state machine and the
// log buffer. See docs/ipc-api.md.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	shutdownTimeout = 5 * time.Second
	// maxBodyBytes caps request bodies. A config is a few KiB.
	maxBodyBytes = 1 << 20
)

// Options configures a Server.
type Options struct {
	// Version is the service build version.
	Version string
	// Logger receives service logs. If nil, slog.Default is used.
	Logger *slog.Logger
	// Engine runs Nebula. Required.
	Engine Engine
	// Store holds the imported config. Required.
	Store *Store
	// Logs is served by GET /v1/logs. If nil, an empty buffer is used.
	Logs *LogBuffer
}

// Server serves the API.
type Server struct {
	version string
	log     *slog.Logger
	mux     *http.ServeMux
	engine  Engine
	mgr     *manager
	logs    *LogBuffer
}

// New returns a Server. It loads the stored config, if any.
func New(opts Options) (*Server, error) {
	if opts.Engine == nil || opts.Store == nil {
		return nil, errors.New("service: Engine and Store are required")
	}
	s := &Server{
		version: opts.Version,
		log:     opts.Logger,
		mux:     http.NewServeMux(),
		engine:  opts.Engine,
		logs:    opts.Logs,
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.logs == nil {
		s.logs = NewLogBuffer(0)
	}
	mgr, err := newManager(opts.Engine, opts.Store, s.log)
	if err != nil {
		return nil, err
	}
	s.mgr = mgr

	s.mux.HandleFunc("GET /v1/version", s.handleVersion)
	s.mux.HandleFunc("GET /v1/status", s.handleStatus)
	s.mux.HandleFunc("PUT /v1/config", s.handlePutConfig)
	s.mux.HandleFunc("GET /v1/config", s.handleGetConfig)
	s.mux.HandleFunc("DELETE /v1/config", s.handleDeleteConfig)
	s.mux.HandleFunc("POST /v1/connect", s.handleConnect)
	s.mux.HandleFunc("POST /v1/disconnect", s.handleDisconnect)
	s.mux.HandleFunc("GET /v1/logs", s.handleLogs)
	return s, nil
}

// Close stops Nebula if it is running. Serve calls it on shutdown.
func (s *Server) Close() { s.mgr.shutdown() }

// Handler returns the API's HTTP handler.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.mux.ServeHTTP(w, r)
		s.log.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// Serve serves the API on l until ctx is cancelled, then shuts down
// gracefully and stops Nebula. It closes l.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	defer s.Close()
	hs := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(l) }()
	s.log.Info("service listening", "addr", l.Addr().String(), "version", s.version)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	s.log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handleVersion(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, api.VersionResponse{
		ServiceVersion: s.version,
		NebulaVersion:  s.engine.Version(),
		APIVersion:     api.APIVersion,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.mgr.status())
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var req api.ConfigRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			s.writeError(w, api.CodeBadRequest, "request body too large")
			return
		}
		s.writeError(w, api.CodeBadRequest, "request body is not valid JSON")
		return
	}
	if req.YAML == "" {
		s.writeError(w, api.CodeBadRequest, "yaml is required")
		return
	}
	restart, err := s.mgr.putConfig(req.YAML)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.log.Info("config imported", "restart_required", restart)
	s.writeJSON(w, http.StatusOK, api.ConfigPutResponse{RestartRequired: restart})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	yaml, err := s.mgr.config()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, api.ConfigResponse{YAML: yaml})
}

func (s *Server) handleDeleteConfig(w http.ResponseWriter, _ *http.Request) {
	if err := s.mgr.deleteConfig(); err != nil {
		s.writeErr(w, err)
		return
	}
	s.log.Info("config deleted")
	s.writeJSON(w, http.StatusOK, s.mgr.status())
}

func (s *Server) handleConnect(w http.ResponseWriter, _ *http.Request) {
	if err := s.mgr.connect(); err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, s.mgr.status())
}

func (s *Server) handleDisconnect(w http.ResponseWriter, _ *http.Request) {
	if err := s.mgr.disconnect(); err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, s.mgr.status())
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	var since uint64
	if v := r.URL.Query().Get("since"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			s.writeError(w, api.CodeBadRequest, "since must be a non-negative integer")
			return
		}
		since = n
	}
	s.writeJSON(w, http.StatusOK, s.logs.Since(since))
}

// writeErr maps an error from the manager to an API error. Unknown errors
// are logged and reported as internal without detail.
func (s *Server) writeErr(w http.ResponseWriter, err error) {
	var ic *errInvalidConfig
	var se *errStart
	switch {
	case errors.As(err, &ic):
		s.writeReasonError(w, api.CodeInvalidConfig, ic.reason, ic.msg)
	case errors.Is(err, errNotConfigured):
		s.writeError(w, api.CodeNotConfigured, err.Error())
	case errors.Is(err, errAlreadyRunning):
		s.writeError(w, api.CodeAlreadyRunning, err.Error())
	case errors.Is(err, errNotRunning):
		s.writeError(w, api.CodeNotRunning, err.Error())
	case errors.As(err, &se):
		s.writeReasonError(w, api.CodeInternal, se.reason, se.Error())
	default:
		s.log.Error("request failed", "err", err)
		s.writeError(w, api.CodeInternal, "internal error, see the service log")
	}
}

func (s *Server) writeError(w http.ResponseWriter, code api.ErrorCode, msg string) {
	s.writeReasonError(w, code, "", msg)
}

func (s *Server) writeReasonError(w http.ResponseWriter, code api.ErrorCode, reason api.Reason, msg string) {
	s.writeJSON(w, httpStatus(code), api.ErrorBody{Error: &api.Error{Code: code, Reason: reason, Message: msg}})
}

func httpStatus(code api.ErrorCode) int {
	switch code {
	case api.CodeBadRequest:
		return http.StatusBadRequest
	case api.CodeInvalidConfig:
		return http.StatusUnprocessableEntity
	case api.CodeNotConfigured, api.CodeAlreadyRunning, api.CodeNotRunning:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Warn("write response", "err", err)
	}
}
