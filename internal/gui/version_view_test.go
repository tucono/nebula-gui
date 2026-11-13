// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/tucono/nebula-gui/internal/api"
)

func TestDescribeVersion(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "unix", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	tests := []struct {
		name     string
		v        *api.VersionResponse
		err      error
		ok       bool
		versions bool
		problem  problemID
	}{
		{"unreachable", nil, refused, false, false, probServiceNotRunning},
		{"match", &api.VersionResponse{ServiceVersion: "0.1.0", APIVersion: api.APIVersion}, nil, true, true, ""},
		{"service newer", &api.VersionResponse{APIVersion: api.APIVersion + 1}, nil, false, true, probAppTooOld},
		{"service older", &api.VersionResponse{APIVersion: api.APIVersion - 1}, nil, false, true, probServiceTooOld},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeVersion(tt.v, tt.err)
			var id problemID
			if got.Problem != nil {
				id = got.Problem.ID
			}
			if got.OK != tt.ok || got.ShowVersions != tt.versions || id != tt.problem {
				t.Fatalf("got %+v (problem %q)", got, id)
			}
		})
	}
}

func TestDescribeVersionEmptyNebula(t *testing.T) {
	got := describeVersion(&api.VersionResponse{ServiceVersion: "0.1.0", APIVersion: api.APIVersion}, nil)
	if got.NebulaVer != "—" {
		t.Fatalf("NebulaVer = %q, want dash", got.NebulaVer)
	}
}
