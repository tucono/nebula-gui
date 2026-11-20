// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// removeAutostartValues removes u's Run and StartupApproved values from
// their registry hive. A signed-in user's hive is already loaded under
// HKEY_USERS\<SID>; anyone else's is loaded from NTUSER.DAT in their
// profile folder for the change and unloaded again. A profile without
// one has never been used, so has nothing to remove.
func removeAutostartValues(u User) ([]string, error) {
	if u.SID == "" {
		return nil, nil
	}
	label := `HKEY_USERS\` + u.SID
	k, err := registry.OpenKey(registry.USERS, u.SID, registry.QUERY_VALUE)
	if err == nil {
		defer func() { _ = k.Close() }()
		return removeRunValues(k, label)
	}
	hive := filepath.Join(u.Home, "NTUSER.DAT")
	if _, err := os.Lstat(hive); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var removed []string
	err = withHive(hive, hiveKeyPrefix+u.SID, func(k registry.Key) error {
		var err error
		removed, err = removeRunValues(k, label)
		return err
	})
	return removed, err
}

// hiveKeyPrefix names the HKEY_USERS key a hive is loaded under while it's
// changed, so it can't clash with a user signing in.
const hiveKeyPrefix = "nebula-gui-uninstall-"

// removeRunValues removes the app's value under RunKey and ApprovedKey in
// a user's hive k, naming each it removed under label. Values or keys
// that aren't there aren't errors.
func removeRunValues(k registry.Key, label string) ([]string, error) {
	var removed []string
	var errs []error
	for _, path := range []string{RunKey, ApprovedKey} {
		name := label + `\` + path + `\` + RunValue
		sk, err := registry.OpenKey(k, path, registry.SET_VALUE)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("open %s: %w", label+`\`+path, err))
			continue
		}
		err = sk.DeleteValue(RunValue)
		_ = sk.Close()
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			errs = append(errs, fmt.Errorf("remove %s: %w", name, err))
		default:
			removed = append(removed, name)
		}
	}
	return removed, errors.Join(errs...)
}
