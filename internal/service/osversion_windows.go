// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// osVersion returns the registry's ProductName and DisplayVersion, for
// example "Windows 11 Pro 24H2", or "" if they can't be read.
func osVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	name, _, _ := k.GetStringValue("ProductName")
	// Windows 11 still says "Windows 10" in ProductName; its builds start
	// at 22000.
	if build, _, err := k.GetStringValue("CurrentBuildNumber"); err == nil {
		if n, err := strconv.Atoi(build); err == nil && n >= 22000 {
			name = strings.Replace(name, "Windows 10", "Windows 11", 1)
		}
	}
	ver, _, _ := k.GetStringValue("DisplayVersion")
	return strings.TrimSpace(name + " " + ver)
}
