//go:build !linux

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import "errors"

// removeEverything is Linux's: the macOS and Windows uninstallers ask.
func removeEverything([]string) error {
	return errors.New("remove-everything is for Linux; use the uninstaller, which asks whether to remove everything")
}
