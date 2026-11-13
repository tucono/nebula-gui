// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"
	"strings"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// action is a change the user asked the service for.
type action int

const (
	actNone action = iota
	actConnect
	actDisconnect
	// actReconnect is Disconnect then Connect, to use a new config.
	actReconnect
	// actRemoveConfig deletes the stored config.
	actRemoveConfig
)

func (a action) String() string {
	switch a {
	case actConnect:
		return "connect"
	case actDisconnect:
		return "disconnect"
	case actReconnect:
		return "reconnect"
	case actRemoveConfig:
		return "remove config"
	default:
		return "none"
	}
}

// op is what the action means for choosing a problem.
func (a action) op() op {
	if a == actRemoveConfig {
		return opOther
	}
	return opConnect
}

// statusInput is everything the window knows about the service.
type statusInput struct {
	// Status is the latest status, nil before the first answer and while
	// the service can't be reached.
	Status *api.StatusResponse
	// Err is set while the service can't be reached.
	Err error
	// Version is set while the service speaks another API version.
	Version *problem
	// Action is the problem from the last failed Connect or Disconnect.
	// It stays until the user acts again, dismisses it, or the connection
	// starts.
	Action *problem
	// NewConfig is set when an import replaced the config of a running
	// connection, until the connection stops.
	NewConfig bool
	// Acting is the action in flight.
	Acting action
	Now    time.Time
	GOOS   string
}

// statusView is what the window shows. It is plain data so it can be
// tested without Fyne.
type statusView struct {
	// Banner is the problem shown at the top, or nil.
	Banner *problem
	// Waiting is set before the service has answered for the first time.
	Waiting bool
	// ShowHeader shows the status header. It is hidden while the service
	// can't be used, and while the banner shows the error state.
	ShowHeader bool
	Headline   string
	// Busy shows a spinner next to the headline.
	Busy bool

	Address, Cert, ValidUntil, Uptime string

	Button        string
	ButtonAction  action
	ButtonEnabled bool
	// ManageEnabled enables Import config… and Remove config….
	ManageEnabled bool
}

// describeStatus turns what the window knows into what it shows. Problems
// with the service come first, then the empty state, then the problem from
// the last action, then problems the status shows by itself.
func describeStatus(in statusInput) statusView {
	switch {
	case in.Err != nil:
		return statusView{Banner: problemFor(in.Err, problemContext{GOOS: in.GOOS})}
	case in.Version != nil:
		return statusView{Banner: in.Version}
	case in.Status == nil:
		return statusView{Waiting: true}
	case !in.Status.Configured:
		return statusView{Banner: noConfig()}
	}
	st := in.Status

	v := statusView{Banner: in.Action}
	if v.Banner == nil {
		v.Banner = statusProblem(st, in.GOOS)
	}
	if v.Banner == nil && in.NewConfig && st.State == api.StateRunning {
		v.Banner = newConfigSaved()
	}
	if st.State == api.StateError {
		// The banner says what went wrong and offers Connect again and
		// Dismiss; a header saying "Not connected" would repeat it.
		return v
	}

	v.ShowHeader = true
	v.Address = orDash(strings.Join(st.OverlayIPs, ", "))
	v.Cert, v.ValidUntil = "—", "—"
	if st.Cert != nil {
		v.Cert = orDash(st.Cert.Name)
		v.ValidUntil = formatDate(st.Cert.NotAfter)
		if st.Cert.Expired {
			v.ValidUntil += " (expired)"
		}
	}
	v.Uptime = "—"
	if st.State == api.StateRunning && st.StartedAt != nil {
		v.Uptime = formatUptime(in.Now.Sub(*st.StartedAt))
	}

	switch st.State {
	case api.StateRunning:
		v.Headline, v.Button, v.ButtonAction, v.ButtonEnabled = "Connected", "Disconnect", actDisconnect, true
	case api.StateStarting:
		v.Headline, v.Button, v.ButtonAction, v.Busy = "Connecting…", "Connect", actConnect, true
	case api.StateStopping:
		v.Headline, v.Button, v.ButtonAction, v.Busy = "Disconnecting…", "Disconnect", actDisconnect, true
	default:
		v.Headline, v.Button, v.ButtonAction, v.ButtonEnabled = "Not connected", "Connect", actConnect, true
	}

	// While an action is in flight the button stays disabled, and the
	// headline says what's happening even before the status catches up.
	v.ManageEnabled = in.Acting == actNone
	switch in.Acting {
	case actNone:
	case actRemoveConfig:
		v.Headline, v.Busy, v.ButtonEnabled = "Removing the config…", true, false
	case actDisconnect:
		v.Headline, v.Busy, v.ButtonEnabled = "Disconnecting…", true, false
	default:
		v.Headline, v.Busy, v.ButtonEnabled = "Connecting…", true, false
	}
	return v
}

// formatUptime says how long the connection has been up, to the minute:
// "less than a minute", "5 minutes", "2 hours 5 minutes", "3 days 4 hours".
func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	days := int(d / (24 * time.Hour))
	hours := int(d/time.Hour) % 24
	mins := int(d/time.Minute) % 60
	switch {
	case days > 0:
		return joinNonEmpty(plural(days, "day"), plural(hours, "hour"))
	case hours > 0:
		return joinNonEmpty(plural(hours, "hour"), plural(mins, "minute"))
	default:
		return plural(mins, "minute")
	}
}

func plural(n int, unit string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 " + unit
	default:
		return fmt.Sprintf("%d %ss", n, unit)
	}
}

func joinNonEmpty(a, b string) string {
	if b == "" {
		return a
	}
	return a + " " + b
}
