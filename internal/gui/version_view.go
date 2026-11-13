// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"fmt"

	"github.com/tucono/nebula-gui/internal/api"
)

// versionView is what the window shows after asking the service for its
// version. It is plain data so it can be tested without Fyne.
type versionView struct {
	// Problem is set when the service can't be reached or speaks another
	// API version.
	Problem      *problem
	OK           bool // the service answered and speaks our API version
	ServiceVer   string
	NebulaVer    string
	APIVer       string
	ShowVersions bool
}

func describeVersion(v *api.VersionResponse, err error) versionView {
	if err != nil {
		return versionView{Problem: problemFor(err, problemContext{})}
	}
	return versionView{
		Problem:      versionProblem(v),
		OK:           v.APIVersion == api.APIVersion,
		ServiceVer:   orDash(v.ServiceVersion),
		NebulaVer:    orDash(v.NebulaVersion),
		APIVer:       fmt.Sprint(v.APIVersion),
		ShowVersions: true,
	}
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
