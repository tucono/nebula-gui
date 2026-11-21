// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build !dev

package main

import "flag"

// devBuild is false: this is a release build.
const devBuild = false

// devFlag returns false: release builds have no --dev flag.
func devFlag(*flag.FlagSet) *bool { return new(bool) }

func devSocketOwner() string { return "" }
