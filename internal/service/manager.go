// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

var (
	errNotConfigured  = errors.New("no config has been imported")
	errAlreadyRunning = errors.New("nebula is already running")
	errNotRunning     = errors.New("nebula is not running")
)

// errStart wraps a failure to start Nebula.
type errStart struct {
	err    error
	reason api.Reason // optional
}

func (e *errStart) Error() string { return "start nebula: " + e.err.Error() }
func (e *errStart) Unwrap() error { return e.err }

// manager owns the stored config and the Nebula instance. Its state machine
// (docs/architecture.md):
//
//	stopped ──connect──► starting ──ok──► running ──disconnect──► stopping ──► stopped
//	                        │                │
//	                        └──error──► error ◄──nebula exits on its own
//
// From error, connect starts again and disconnect returns to stopped.
type manager struct {
	engine Engine
	store  *Store
	log    *slog.Logger

	// op serializes the operations that change state (connect, disconnect,
	// config changes), which can take a while. Status reads don't take it.
	op sync.Mutex

	// now is the clock for cert expiry; tests replace it.
	now func() time.Time

	mu         sync.Mutex
	state      api.State
	lastErr    string
	lastReason api.Reason
	inst       Instance
	startedAt  time.Time
	configured bool
	stored     *ConfigInfo // the stored config; nil if none or unreadable
	running    *ConfigInfo // the config Nebula was started with
}

func newManager(engine Engine, store *Store, log *slog.Logger) (*manager, error) {
	m := &manager{engine: engine, store: store, log: log, state: api.StateStopped, now: time.Now}
	yaml, ok, err := store.Load()
	if err != nil {
		return nil, err
	}
	if ok {
		m.configured = true
		info, err := engine.Check(yaml)
		if err != nil {
			// Keep it: connect will report the problem, and a new import
			// replaces it. The description, if any, still goes into the
			// status, so for example an expired cert shows up right away.
			log.Warn("stored config does not pass the dry run", "err", err, "reason", reasonOf(err))
		}
		m.stored = info
	}
	return m, nil
}

func (m *manager) status() api.StatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := api.StatusResponse{
		State:       m.state,
		Error:       m.lastErr,
		ErrorReason: m.lastReason,
		OverlayIPs:  []string{},
		Configured:  m.configured,
	}
	info := m.stored
	if m.state == api.StateStarting || m.state == api.StateRunning || m.state == api.StateStopping {
		info = m.running
		if !m.startedAt.IsZero() {
			t := m.startedAt
			s.StartedAt = &t
		}
	}
	if info != nil {
		c := info.Cert
		now := m.now()
		c.Expired = now.After(c.NotAfter)
		c.CAExpired = c.CANotAfter != nil && now.After(*c.CANotAfter)
		s.Cert = &c
		s.OverlayIPs = append(s.OverlayIPs, info.OverlayIPs...)
		s.ListenPort = info.ListenPort
	}
	return s
}

// putConfig validates and stores yaml. It reports whether Nebula is running
// with an older config and needs a restart to pick this one up.
func (m *manager) putConfig(yaml string) (restartRequired bool, err error) {
	if err := validateConfig(yaml); err != nil {
		return false, err
	}
	info, err := m.engine.Check(yaml)
	if err != nil {
		return false, &errInvalidConfig{msg: "nebula rejected the config: " + err.Error(), reason: reasonOf(err), err: err}
	}

	m.op.Lock()
	defer m.op.Unlock()
	if err := m.store.Save(yaml); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configured = true
	m.stored = info
	return m.state == api.StateRunning, nil
}

// config returns the stored config with secrets redacted.
func (m *manager) config() (string, error) {
	yaml, ok, err := m.store.Load()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errNotConfigured
	}
	return redactConfig(yaml)
}

// deleteConfig stops Nebula if it is running and deletes the stored config.
func (m *manager) deleteConfig() error {
	m.op.Lock()
	defer m.op.Unlock()
	m.stopLocked()
	if err := m.store.Delete(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configured = false
	m.stored = nil
	if m.state == api.StateError {
		m.state, m.lastErr, m.lastReason = api.StateStopped, "", ""
	}
	return nil
}

func (m *manager) connect() error {
	m.op.Lock()
	defer m.op.Unlock()

	m.mu.Lock()
	if m.state == api.StateRunning {
		m.mu.Unlock()
		return errAlreadyRunning
	}
	m.mu.Unlock()

	yaml, ok, err := m.store.Load()
	if err != nil {
		return err
	}
	if !ok {
		return errNotConfigured
	}

	m.mu.Lock()
	m.state, m.lastErr, m.lastReason, m.running = api.StateStarting, "", "", m.stored
	m.mu.Unlock()
	m.log.Info("starting nebula")

	inst, err := m.engine.Start(yaml)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		reason := reasonOf(err)
		if configReasons[reason] {
			// The config itself is the problem (typically a cert that has
			// expired since the import). Nebula never ran, so there is no
			// error state to clear; the status shows the expiry.
			m.state, m.running = api.StateStopped, nil
			m.log.Warn("nebula refused the config", "err", err, "reason", reason)
			return &errInvalidConfig{msg: "nebula rejected the config: " + err.Error(), reason: reason, err: err}
		}
		m.state, m.lastErr, m.lastReason, m.running = api.StateError, err.Error(), reason, nil
		m.log.Error("nebula failed to start", "err", err, "reason", reason)
		return &errStart{err: err, reason: reason}
	}
	m.state, m.inst, m.startedAt = api.StateRunning, inst, time.Now()
	go m.watch(inst)
	m.log.Info("nebula running")
	return nil
}

// watch moves to the error state if inst stops without being asked to.
func (m *manager) watch(inst Instance) {
	<-inst.Done()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inst != inst || m.state != api.StateRunning {
		return // stopped on purpose
	}
	msg := "nebula stopped unexpectedly"
	if err := inst.Err(); err != nil {
		msg = fmt.Sprintf("%s: %v", msg, err)
	}
	m.state, m.lastErr, m.lastReason = api.StateError, msg, api.ReasonNebulaStopped
	m.inst, m.running, m.startedAt = nil, nil, time.Time{}
	m.log.Error(msg)
}

func (m *manager) disconnect() error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	switch m.state {
	case api.StateRunning:
		m.mu.Unlock()
		m.stopLocked()
		return nil
	case api.StateError:
		m.state, m.lastErr, m.lastReason = api.StateStopped, "", ""
		m.mu.Unlock()
		return nil
	default:
		m.mu.Unlock()
		return errNotRunning
	}
}

// shutdown stops Nebula when the service exits.
func (m *manager) shutdown() {
	m.op.Lock()
	defer m.op.Unlock()
	m.stopLocked()
}

// stopLocked stops Nebula if it is running. The caller holds m.op.
func (m *manager) stopLocked() {
	m.mu.Lock()
	if m.state != api.StateRunning {
		m.mu.Unlock()
		return
	}
	inst := m.inst
	m.state = api.StateStopping
	m.mu.Unlock()

	m.log.Info("stopping nebula")
	inst.Stop()

	m.mu.Lock()
	m.state, m.inst, m.running, m.startedAt = api.StateStopped, nil, nil, time.Time{}
	m.mu.Unlock()
	m.log.Info("nebula stopped")
}

// reasonOf returns the reason nebulactl found for err, if any.
func reasonOf(err error) api.Reason { return api.Reason(nebulactl.ReasonOf(err)) }

// configReasons are the failures caused by the config itself. When Start
// reports one, connect answers invalid_config instead of internal.
var configReasons = map[api.Reason]bool{
	api.ReasonCertExpired:  true,
	api.ReasonCAExpired:    true,
	api.ReasonKeyMismatch:  true,
	api.ReasonWrongCA:      true,
	api.ReasonKeyEncrypted: true,
}
