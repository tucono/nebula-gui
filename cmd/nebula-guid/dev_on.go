// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build dev

package main

import "flag"

// devBuild is true in builds made with -tags dev.
const devBuild = true

// devFlag registers --dev. It exists only in builds made with -tags dev.
func devFlag(fs *flag.FlagSet) *bool {
	return fs.Bool("dev", false, "development mode: debug logging, and give the launching user access to the socket")
}
