// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"slices"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/tucono/nebula-gui/internal/api"
)

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	n := api.DefaultDiagnosticsLines
	if v := r.URL.Query().Get("lines"); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil || i < 1 {
			s.writeError(w, api.CodeBadRequest, "lines must be a positive integer")
			return
		}
		n = min(i, api.MaxDiagnosticsLines)
	}

	resp := api.DiagnosticsResponse{
		ServiceVersion: s.version,
		NebulaVersion:  s.engine.Version(),
		APIVersion:     api.APIVersion,
		InstanceID:     s.instanceID,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		OSVersion:      s.osVersion,
		Status:         s.status(),
		Log:            []string{},
	}
	if raw, ok, err := s.mgr.store.Load(); err != nil {
		s.log.Warn("diagnostics: read stored config", "err", err)
	} else if ok {
		resp.ConfigSummary = summarizeConfig(raw)
	}
	if s.logFile != nil {
		lines, truncated, err := s.logFile.Tail(n)
		if err != nil {
			s.log.Warn("diagnostics: read log file", "err", err)
		} else {
			resp.Log, resp.LogTruncated = lines, truncated
		}
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// summarizeConfig describes raw with counts, flags and addresses only, or
// returns nil if raw doesn't parse. Unexpected shapes count as absent: the
// summary is for troubleshooting, and Nebula's dry run is what validates.
func summarizeConfig(raw string) *api.ConfigSummary {
	var c map[string]any
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil || c == nil {
		return nil
	}
	sum := &api.ConfigSummary{Lighthouses: []string{}, StaticHostNames: []string{}}

	hostMap, _ := c["static_host_map"].(map[string]any)
	lh, _ := c["lighthouse"].(map[string]any)
	sum.AmLighthouse = lh["am_lighthouse"] == true
	hosts, _ := lh["hosts"].([]any)
	for _, h := range hosts {
		for _, addr := range stringList(hostMap[toString(h)]) {
			if !slices.Contains(sum.Lighthouses, addr) {
				sum.Lighthouses = append(sum.Lighthouses, addr)
			}
		}
	}
	for _, addrs := range hostMap {
		for _, addr := range stringList(addrs) {
			host := addr
			if h, _, err := net.SplitHostPort(addr); err == nil {
				host = h
			}
			if _, err := netip.ParseAddr(host); err == nil || host == "" {
				continue
			}
			if !slices.Contains(sum.StaticHostNames, host) {
				sum.StaticHostNames = append(sum.StaticHostNames, host)
			}
		}
	}
	slices.Sort(sum.StaticHostNames)

	relay, _ := c["relay"].(map[string]any)
	sum.Relays = len(asList(relay["relays"])) > 0

	switch p := c["punchy"].(type) {
	case bool: // the old form, punchy: true
		sum.Punchy = p
	case map[string]any:
		sum.Punchy = p["punch"] == true
	}

	listen, _ := c["listen"].(map[string]any)
	sum.ListenPort, _ = listen["port"].(int)

	tun, _ := c["tun"].(map[string]any)
	sum.TunRoutes = len(asList(tun["routes"]))
	sum.UnsafeRoutes = len(asList(tun["unsafe_routes"]))

	fw, _ := c["firewall"].(map[string]any)
	sum.FirewallInbound = len(asList(fw["inbound"]))
	sum.FirewallOutbound = len(asList(fw["outbound"]))
	return sum
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// stringList returns v as a list of strings: a single string, or the
// strings in a list.
func stringList(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
