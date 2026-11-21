//go:build !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import "os"

// restrictDir makes dir accessible to its owner only.
func restrictDir(dir string) error {
	return os.Chmod(dir, 0o700) //nolint:gosec // a directory needs the x bit
}
