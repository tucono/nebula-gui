// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// settingsFile is the service settings' name inside the state directory.
const settingsFile = "settings.json"

// Autoconnect retries. A failed start at boot is retried, because the
// network may not be ready yet on Windows and macOS (docs/m4-plan.md).
const (
	autoconnectRetries = 3
	autoconnectDelay   = 30 * time.Second
)

// settingsStore holds the service settings in memory and in settings.json.
type settingsStore struct {
	store *Store

	mu  sync.Mutex // held across a save, so saves don't interleave
	cur api.Settings
}

// loadSettings reads settings.json. A missing file means the defaults. A
// file that can't be read or parsed also means the defaults, with a
// warning: settings must never stop the service from running.
func loadSettings(store *Store, log *slog.Logger) *settingsStore {
	ss := &settingsStore{store: store}
	b, err := os.ReadFile(filepath.Join(store.Dir(), settingsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return ss
	}
	if err == nil {
		err = json.Unmarshal(b, &ss.cur)
	}
	if err != nil {
		ss.cur = api.Settings{}
		log.Warn("settings file is damaged, using the defaults", "err", err)
	}
	return ss
}

func (ss *settingsStore) get() api.Settings {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.cur
}

// set saves s and then makes it current. If the save fails, nothing
// changes.
func (ss *settingsStore) set(s api.Settings) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := ss.store.writeFile(settingsFile, b); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	ss.cur = s
	return nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.settings.get())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req api.SettingsRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		s.writeError(w, api.CodeBadRequest, "request body is not valid settings: "+err.Error())
		return
	}
	if req.Autoconnect == nil {
		s.writeError(w, api.CodeBadRequest, "autoconnect is required")
		return
	}
	next := api.Settings{Autoconnect: *req.Autoconnect}
	if err := s.settings.set(next); err != nil {
		s.writeErr(w, err)
		return
	}
	s.log.Info("settings changed", "autoconnect", next.Autoconnect)
	s.writeJSON(w, http.StatusOK, next)
}

// autoconnect connects at start-up if the setting is on and a config is
// stored. A failure to start Nebula is retried after autoconnectDelay, up
// to autoconnectRetries times, as long as the setting is still on and the
// state is still error: a Connect or Disconnect from the user in the
// meantime ends the retries. It returns when done or when ctx ends.
func (s *Server) autoconnect(ctx context.Context) {
	for attempt := 0; ; attempt++ {
		if !s.settings.get().Autoconnect || !s.mgr.status().Configured {
			return
		}
		if attempt == 0 {
			s.log.Info("connecting automatically")
		} else {
			s.log.Info("connecting automatically, trying again", "attempt", attempt+1)
		}
		err := s.mgr.connect()
		var se *errStart
		switch {
		case err == nil, errors.Is(err, errAlreadyRunning):
			return
		case !errors.As(err, &se):
			// The config was rejected, or the store can't be read.
			// Trying again won't help; the manager has logged why.
			s.log.Warn("not connecting automatically", "err", err)
			return
		case attempt == autoconnectRetries:
			s.log.Error("giving up connecting automatically", "attempts", attempt+1)
			return
		}
		if !s.sleep(ctx, s.autoconnectDelay) {
			return
		}
		if s.mgr.status().State != api.StateError {
			return
		}
	}
}

// sleepCtx waits for d, or until ctx ends. It reports whether d passed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
