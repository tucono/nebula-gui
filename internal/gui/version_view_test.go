// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"strings"
	"testing"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestDescribeVersion(t *testing.T) {
	tests := []struct {
		name     string
		v        *api.VersionResponse
		err      error
		ok       bool
		versions bool
		contains string
	}{
		{"unreachable", nil, errors.New("connection refused"), false, false, "Can't reach"},
		{"match", &api.VersionResponse{ServiceVersion: "0.1.0", APIVersion: api.APIVersion}, nil, true, true, "Connected"},
		{"service newer", &api.VersionResponse{APIVersion: api.APIVersion + 1}, nil, false, true, "Update the app"},
		{"service older", &api.VersionResponse{APIVersion: api.APIVersion - 1}, nil, false, true, "Update the service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeVersion("/tmp/s.sock", tt.v, tt.err)
			if got.OK != tt.ok || got.ShowVersions != tt.versions || !strings.Contains(got.Message, tt.contains) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestDescribeVersionEmptyNebula(t *testing.T) {
	got := describeVersion("x", &api.VersionResponse{ServiceVersion: "0.1.0", APIVersion: api.APIVersion}, nil)
	if got.NebulaVer != "—" {
		t.Fatalf("NebulaVer = %q, want dash", got.NebulaVer)
	}
}
