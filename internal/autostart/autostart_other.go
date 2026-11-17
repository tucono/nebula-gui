//go:build (!linux && !darwin && !windows) || android || ios

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

func newEntry(string) (Entry, error) { return nil, ErrUnsupported }
