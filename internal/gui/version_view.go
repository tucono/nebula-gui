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
	Message      string // headline: connected, unreachable, update required
	OK           bool   // the service answered and speaks our API version
	ServiceVer   string
	NebulaVer    string
	APIVer       string
	ShowVersions bool
}

func describeVersion(addr string, v *api.VersionResponse, err error) versionView {
	if err != nil {
		return versionView{
			Message: fmt.Sprintf("Can't reach the nebula-guid service at %s.\n%v", addr, err),
		}
	}
	vv := versionView{
		ServiceVer:   orDash(v.ServiceVersion),
		NebulaVer:    orDash(v.NebulaVersion),
		APIVer:       fmt.Sprint(v.APIVersion),
		ShowVersions: true,
	}
	switch {
	case v.APIVersion == api.APIVersion:
		vv.OK = true
		vv.Message = "Connected to the service."
	case v.APIVersion > api.APIVersion:
		vv.Message = fmt.Sprintf("The service uses API version %d, but this app only supports %d. Update the app.",
			v.APIVersion, api.APIVersion)
	default:
		vv.Message = fmt.Sprintf("The service uses API version %d, but this app needs %d. Update the service.",
			v.APIVersion, api.APIVersion)
	}
	return vv
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
