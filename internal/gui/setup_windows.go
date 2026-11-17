//go:build windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// probeSetup asks the Service Control Manager whether the service is
// registered. Both calls need only the rights every user has.
func probeSetup() serviceSetup {
	return windowsSetup(windowsService)
}

func windowsSetup(name string) serviceSetup {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return serviceSetup{}
	}
	defer func() { _ = windows.CloseServiceHandle(m) }()
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return serviceSetup{}
	}
	s, err := windows.OpenService(m, n, windows.SERVICE_QUERY_STATUS)
	switch {
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		return serviceSetup{Known: true}
	case err != nil:
		return serviceSetup{}
	}
	_ = windows.CloseServiceHandle(s)
	return serviceSetup{Known: true, Installed: true, CanStart: true}
}

// startService runs sc start elevated, so Windows asks to allow it. It
// returns once sc has started; the service comes up a moment later.
func startService(context.Context) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(filepath.Join(dir, "sc.exe"))
	args, _ := windows.UTF16PtrFromString("start " + windowsService)
	err = windows.ShellExecute(0, verb, file, args, nil, windows.SW_HIDE)
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return fmt.Errorf("%w: %v", errStartCancelled, err)
	}
	if err != nil {
		return fmt.Errorf("sc start %s: %w", windowsService, err)
	}
	return nil
}
