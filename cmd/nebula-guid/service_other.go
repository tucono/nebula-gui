// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

//go:build !windows

package main

import "errors"

// errWindowsOnly answers the Windows commands on other systems, where the
// package's systemd unit or launchd job runs the service in the foreground.
var errWindowsOnly = errors.New("install, uninstall and run are only for Windows; " +
	"on Linux and macOS the package sets up the service, which runs nebula-guid without a command")

func installService([]string) error   { return errWindowsOnly }
func uninstallService([]string) error { return errWindowsOnly }
func runService([]string) error       { return errWindowsOnly }
