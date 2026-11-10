// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build !dev

package main

// devFlag returns false: release builds have no --dev flag.
func devFlag() *bool { return new(bool) }

func devSocketOwner() string { return "" }
