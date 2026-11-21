//go:build unix

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// writeFile replaces path with data, creating its directory. It writes a
// temporary file and renames it, so a crash leaves the old file or the
// new one.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // the autostart directory is the desktop's, readable as usual
		return fmt.Errorf("create %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".nebula-gui-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// removeFile removes path; a file that's already gone is fine.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
