// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"testing"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestDescribeStatus(t *testing.T) {
	now := time.Date(2026, 11, 13, 14, 0, 0, 0, time.Local)
	started := now.Add(-(2*time.Hour + 5*time.Minute))
	cert := &api.CertInfo{Name: "dev", NotAfter: time.Date(2027, 3, 3, 12, 0, 0, 0, time.Local)}
	expired := &api.CertInfo{Name: "dev", NotAfter: now.Add(-time.Hour), Expired: true}
	st := func(s api.State) *api.StatusResponse {
		r := &api.StatusResponse{State: s, Configured: true, OverlayIPs: []string{"192.168.100.20/24"}, Cert: cert}
		if s == api.StateRunning {
			r.StartedAt = &started
		}
		return r
	}
	failed := connectFailed()

	tests := []struct {
		name     string
		in       statusInput
		banner   problemID
		waiting  bool
		header   bool
		headline string
		busy     bool
		button   string
		act      action
		enabled  bool
	}{
		{name: "nothing yet", in: statusInput{}, waiting: true},
		{name: "unreachable", in: statusInput{Err: errRefused, Status: st(api.StateRunning)}, banner: probServiceNotRunning},
		{name: "incompatible", in: statusInput{Version: appTooOld(2, 1)}, banner: probAppTooOld},
		{name: "no config", in: statusInput{Status: &api.StatusResponse{State: api.StateStopped}}, banner: probNoConfig},
		{name: "stopped", in: statusInput{Status: st(api.StateStopped)},
			header: true, headline: "Not connected", button: "Connect", act: actConnect, enabled: true},
		{name: "running", in: statusInput{Status: st(api.StateRunning)},
			header: true, headline: "Connected", button: "Disconnect", act: actDisconnect, enabled: true},
		{name: "starting", in: statusInput{Status: st(api.StateStarting)},
			header: true, headline: "Connecting…", busy: true, button: "Connect", act: actConnect},
		{name: "stopping", in: statusInput{Status: st(api.StateStopping)},
			header: true, headline: "Disconnecting…", busy: true, button: "Disconnect", act: actDisconnect},
		{name: "connect in flight", in: statusInput{Status: st(api.StateStopped), Acting: actConnect},
			header: true, headline: "Connecting…", busy: true, button: "Connect", act: actConnect},
		{name: "disconnect in flight", in: statusInput{Status: st(api.StateRunning), Acting: actDisconnect},
			header: true, headline: "Disconnecting…", busy: true, button: "Disconnect", act: actDisconnect},
		{name: "error state", in: statusInput{Status: &api.StatusResponse{State: api.StateError, Configured: true, ErrorReason: api.ReasonTunUnavailable}},
			banner: probTunUnavailable},
		{name: "failed action over stopped", in: statusInput{Status: st(api.StateStopped), Action: failed},
			banner: probConnectFailed, header: true, headline: "Not connected", button: "Connect", act: actConnect, enabled: true},
		{name: "new config while running", in: statusInput{Status: st(api.StateRunning), NewConfig: true},
			banner: probNewConfigSaved, header: true, headline: "Connected", button: "Disconnect", act: actDisconnect, enabled: true},
		{name: "new config ignored when stopped", in: statusInput{Status: st(api.StateStopped), NewConfig: true},
			header: true, headline: "Not connected", button: "Connect", act: actConnect, enabled: true},
		{name: "remove in flight", in: statusInput{Status: st(api.StateStopped), Acting: actRemoveConfig},
			header: true, headline: "Removing the config…", busy: true, button: "Connect", act: actConnect},
		{name: "expired cert", in: statusInput{Status: &api.StatusResponse{State: api.StateStopped, Configured: true, Cert: expired}},
			banner: probCertExpired, header: true, headline: "Not connected", button: "Connect", act: actConnect, enabled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Now = now
			v := describeStatus(tt.in)
			var id problemID
			if v.Banner != nil {
				id = v.Banner.ID
			}
			if id != tt.banner || v.Waiting != tt.waiting || v.ShowHeader != tt.header {
				t.Fatalf("banner %q waiting %v header %v", id, v.Waiting, v.ShowHeader)
			}
			if !tt.header {
				return
			}
			if v.Headline != tt.headline || v.Busy != tt.busy || v.Button != tt.button || v.ButtonAction != tt.act || v.ButtonEnabled != tt.enabled {
				t.Fatalf("got %+v", v)
			}
			if v.ManageEnabled != (tt.in.Acting == actNone) {
				t.Fatalf("ManageEnabled %v while acting %v", v.ManageEnabled, tt.in.Acting)
			}
		})
	}
}

func TestDescribeStatusDetails(t *testing.T) {
	now := time.Date(2026, 11, 13, 14, 0, 0, 0, time.Local)
	started := now.Add(-5 * time.Minute)
	v := describeStatus(statusInput{Now: now, Status: &api.StatusResponse{
		State: api.StateRunning, Configured: true, StartedAt: &started,
		OverlayIPs: []string{"192.168.100.20/24"},
		Cert:       &api.CertInfo{Name: "dev", NotAfter: time.Date(2027, 3, 3, 12, 0, 0, 0, time.Local)},
	}})
	if v.Address != "192.168.100.20/24" || v.Cert != "dev" || v.ValidUntil != "3 March 2027" || v.Uptime != "5 minutes" {
		t.Fatalf("got %+v", v)
	}

	v = describeStatus(statusInput{Now: now, Status: &api.StatusResponse{State: api.StateStopped, Configured: true,
		Cert: &api.CertInfo{Name: "dev", NotAfter: time.Date(2026, 3, 3, 12, 0, 0, 0, time.Local), Expired: true}}})
	if v.Address != "—" || v.ValidUntil != "3 March 2026 (expired)" || v.Uptime != "—" {
		t.Fatalf("stopped: %+v", v)
	}
}

func TestStatusProblemDismiss(t *testing.T) {
	p := statusProblem(&api.StatusResponse{State: api.StateError, Configured: true, ErrorReason: api.ReasonNebulaStopped}, "linux")
	if p == nil || !p.Dismiss {
		t.Fatalf("error state can't be dismissed: %+v", p)
	}
	p = statusProblem(&api.StatusResponse{State: api.StateStopped, Configured: true, Cert: &api.CertInfo{Expired: true}}, "linux")
	if p == nil || p.Dismiss {
		t.Fatalf("an expired cert can be dismissed: %+v", p)
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "less than a minute"},
		{59 * time.Second, "less than a minute"},
		{time.Minute, "1 minute"},
		{5*time.Minute + 30*time.Second, "5 minutes"},
		{time.Hour, "1 hour"},
		{2*time.Hour + 5*time.Minute, "2 hours 5 minutes"},
		{25 * time.Hour, "1 day 1 hour"},
		{72*time.Hour + 30*time.Minute, "3 days"},
	}
	for _, tt := range tests {
		if got := formatUptime(tt.d); got != tt.want {
			t.Errorf("formatUptime(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
