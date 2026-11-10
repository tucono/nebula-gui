// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build dev

package main

import "flag"

// devFlag registers --dev. It exists only in builds made with -tags dev.
func devFlag() *bool {
	return flag.Bool("dev", false, "development mode: debug logging, and give the launching user access to the socket")
}
