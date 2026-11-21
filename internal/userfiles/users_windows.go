// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// profileListKey lists every profile on the computer by SID, with its
// folder in ProfileImagePath.
const profileListKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`

// userSIDPrefix starts the SIDs of real accounts, local or domain;
// LocalSystem, LocalService and NetworkService (S-1-5-18 to -20) don't
// have it.
const userSIDPrefix = "S-1-5-21-"

// users reads the profile list, which also holds people who have never
// signed in since a reboot and whose hives aren't loaded.
func users() ([]User, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, profileListKey,
		registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer func() { _ = k.Close() }()
	return profilesIn(k)
}

// profilesIn lists the real accounts' profiles under a ProfileList key.
// Windows leaves a "<SID>.bak" key behind when a profile was rebuilt;
// those and profiles without a folder are skipped.
func profilesIn(k registry.Key) ([]User, error) {
	sids, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	var out []User
	for _, sid := range sids {
		if !strings.HasPrefix(sid, userSIDPrefix) || strings.Contains(sid, ".") {
			continue
		}
		home, err := profileFolder(k, sid)
		if err != nil || home == "" {
			continue
		}
		out = append(out, User{Name: accountName(sid, home), Home: home, SID: sid})
	}
	return out, nil
}

// profileFolder reads a profile's folder, expanding %SystemDrive%.
func profileFolder(k registry.Key, sid string) (string, error) {
	pk, err := registry.OpenKey(k, sid, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer func() { _ = pk.Close() }()
	p, _, err := pk.GetStringValue("ProfileImagePath")
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return registry.ExpandString(p)
}

// accountName names the account for messages, or falls back to its
// folder's name when the account is gone or its domain can't be reached.
func accountName(sid, home string) string {
	s, err := windows.StringToSid(sid)
	if err == nil {
		if name, domain, _, err := s.LookupAccount(""); err == nil && name != "" {
			if domain != "" {
				return domain + `\` + name
			}
			return name
		}
	}
	return filepath.Base(home)
}
