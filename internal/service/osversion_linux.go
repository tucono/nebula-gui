// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// osVersion returns PRETTY_NAME from os-release, for example
// "Ubuntu 24.04.1 LTS", or "" if it can't be read.
func osVersion() string {
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		f, err := os.Open(p) //nolint:gosec // fixed system paths
		if err != nil {
			continue
		}
		v := prettyName(f)
		_ = f.Close()
		return v
	}
	return ""
}

func prettyName(f *os.File) string {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "PRETTY_NAME=")
		if !ok {
			continue
		}
		if u, err := strconv.Unquote(v); err == nil {
			return u
		}
		return strings.Trim(v, `'"`)
	}
	return ""
}
