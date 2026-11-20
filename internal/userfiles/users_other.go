//go:build !linux && !darwin && !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import "errors"

func users() ([]User, error) { return nil, errors.New("listing users isn't supported here") }
