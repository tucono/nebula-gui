// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package service implements the privileged service: the HTTP API served
// over the IPC transport. See docs/ipc-api.md.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

const shutdownTimeout = 5 * time.Second

// Options configures a Server.
type Options struct {
	// Version is the service build version.
	Version string
	// Logger receives service logs. If nil, slog.Default is used.
	Logger *slog.Logger
}

// Server serves the API.
type Server struct {
	version string
	log     *slog.Logger
	mux     *http.ServeMux
}

// New returns a Server.
func New(opts Options) *Server {
	s := &Server{version: opts.Version, log: opts.Logger, mux: http.NewServeMux()}
	if s.log == nil {
		s.log = slog.Default()
	}
	s.mux.HandleFunc("GET /v1/version", s.handleVersion)
	s.mux.HandleFunc("GET /v1/status", s.handleStatus)
	return s
}

// Handler returns the API's HTTP handler.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.mux.ServeHTTP(w, r)
		s.log.Debug("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// Serve serves the API on l until ctx is cancelled, then shuts down
// gracefully. It closes l.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
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
		NebulaVersion:  "", // set once Nebula is embedded (M1)
		APIVersion:     api.APIVersion,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	// No Nebula manager or config store yet (M1).
	s.writeJSON(w, http.StatusOK, api.StatusResponse{
		State:      api.StateStopped,
		OverlayIPs: []string{},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Warn("write response", "err", err)
	}
}
