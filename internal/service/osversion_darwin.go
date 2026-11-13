// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// osVersion returns the product name and version from sw_vers, for example
// "macOS 15.1", or "" if it can't be read.
func osVersion() string {
	name := swVers("-productName")
	ver := swVers("-productVersion")
	return strings.TrimSpace(name + " " + ver)
}

func swVers(flag string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/sw_vers", flag).Output() //nolint:gosec // flag is one of two constants
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
