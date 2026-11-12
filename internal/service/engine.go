// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"log/slog"

	"github.com/tucono/nebula-gui/internal/api"
	"github.com/tucono/nebula-gui/internal/nebulactl"
)

// Engine runs Nebula. The real one wraps internal/nebulactl; tests use a
// fake, so no tun device or privileges are needed.
type Engine interface {
	// Version is the embedded Nebula version.
	Version() string
	// Check dry-runs a config (inline PKI only) and describes it.
	Check(yaml string) (*ConfigInfo, error)
	// Start runs Nebula with a config that passed Check.
	Start(yaml string) (Instance, error)
}

// Instance is a running Nebula.
type Instance interface {
	// Stop shuts Nebula down and returns once it has stopped.
	Stop()
	// Done is closed when Nebula stops, through Stop or a fatal error.
	Done() <-chan struct{}
	// Err is the fatal error that stopped Nebula, if any, once Done is closed.
	Err() error
}

// ConfigInfo is what the status endpoint reports about a config.
type ConfigInfo struct {
	Cert       api.CertInfo
	OverlayIPs []string
	ListenPort int
}

// NebulaEngine is the Engine backed by the embedded Nebula library.
type NebulaEngine struct {
	// BuildVersion is reported by Nebula (for example in its startup log).
	BuildVersion string
	// Logger receives Nebula's logs.
	Logger *slog.Logger
}

// Version implements Engine.
func (NebulaEngine) Version() string { return nebulactl.Version() }

// Check implements Engine.
func (NebulaEngine) Check(yaml string) (*ConfigInfo, error) {
	info, err := nebulactl.Check(yaml)
	if err != nil {
		return nil, err
	}
	ips := make([]string, 0, len(info.Cert.Networks))
	for _, n := range info.Cert.Networks {
		ips = append(ips, n.String())
	}
	return &ConfigInfo{
		Cert: api.CertInfo{
			Name:        info.Cert.Name,
			Groups:      info.Cert.Groups,
			NotAfter:    info.Cert.NotAfter,
			Fingerprint: info.Cert.Fingerprint,
		},
		OverlayIPs: ips,
		ListenPort: info.ListenPort,
	}, nil
}

// Start implements Engine.
func (e NebulaEngine) Start(yaml string) (Instance, error) {
	l := e.Logger
	if l == nil {
		l = slog.Default()
	}
	return nebulactl.Start(yaml, e.BuildVersion, l.With("component", "nebula"))
}
