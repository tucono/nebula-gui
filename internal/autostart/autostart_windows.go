// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

const (
	runKey      = userfiles.RunKey
	approvedKey = userfiles.ApprovedKey
	valueName   = userfiles.RunValue
)

// runValue is the nebula-gui value under HKCU's Run key, which Windows runs
// when the user signs in.
type runValue struct {
	run, approved string
	exe           string
}

func newEntry(exe string) (Entry, error) {
	return &runValue{run: runKey, approved: approvedKey, exe: exe}, nil
}

// Enabled reports whether the Run value exists and Task Manager hasn't
// turned it off.
func (r *runValue) Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, r.run, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = k.Close() }()
	if _, _, err := k.GetStringValue(valueName); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !r.turnedOff(), nil
}

// turnedOff reads Task Manager's record: the first byte is even for on and
// odd for off.
func (r *runValue) turnedOff() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, r.approved, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer func() { _ = k.Close() }()
	b, _, err := k.GetBinaryValue(valueName)
	return err == nil && len(b) > 0 && b[0]&1 == 1
}

func (r *runValue) SetEnabled(on bool) error {
	// Task Manager's record would keep a new value turned off, and is
	// stale once the value is gone.
	if err := deleteValue(r.approved, valueName); err != nil {
		return err
	}
	if !on {
		return deleteValue(r.run, valueName)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, r.run, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetStringValue(valueName, `"`+r.exe+`" `+HiddenFlag)
}

// deleteValue removes a value from an HKCU key; a value or key that's
// already gone is fine.
func deleteValue(key, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
