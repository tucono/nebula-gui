// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// configFile is the stored config's name inside the state directory.
const configFile = "nebula.yml"

// Store keeps the imported config in the service's state directory. The
// directory is readable only by the service (0700, or a SYSTEM and
// Administrators DACL on Windows) because the config holds the private key.
type Store struct {
	dir string
}

// OpenStore creates dir if needed and restricts its permissions.
func OpenStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("state directory not set")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	if err := restrictDir(dir); err != nil {
		return nil, fmt.Errorf("restrict state directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the state directory.
func (s *Store) Dir() string { return s.dir }

// Load returns the stored config, or ok == false if there is none.
func (s *Store) Load() (yaml string, ok bool, err error) {
	b, err := os.ReadFile(filepath.Join(s.dir, configFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read stored config: %w", err)
	}
	return string(b), true, nil
}

// Save replaces the stored config atomically.
func (s *Store) Save(yaml string) error {
	f, err := os.CreateTemp(s.dir, configFile+".tmp*")
	if err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op after a successful rename

	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("save config: %w", err)
	}
	if _, err := f.WriteString(yaml); err != nil {
		_ = f.Close()
		return fmt.Errorf("save config: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("save config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, configFile)); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// Delete removes the stored config. Deleting a missing config is not an
// error.
func (s *Store) Delete() error {
	err := os.Remove(filepath.Join(s.dir, configFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete config: %w", err)
	}
	return nil
}
