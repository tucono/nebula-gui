//go:build !windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

// removeAutostartValues has nothing to do outside Windows: the
// start-at-sign-in entry is a file, which Remove handles.
func removeAutostartValues(User) ([]string, error) { return nil, nil }
