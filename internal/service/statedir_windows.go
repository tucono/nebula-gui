// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"os"
	"path/filepath"
)

// DefaultStateDir is where the installed service keeps its config.
func DefaultStateDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "nebula-gui")
}
