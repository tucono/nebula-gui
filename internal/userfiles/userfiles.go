// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Package userfiles names the files the app keeps for each user: its
// settings folder, Fyne's preferences, its cache folder and the
// start-at-sign-in entry. The app finds them for the user running it; an
// uninstaller finds them for every user from their home folders, with
// For. Keeping both here means the two can't drift apart.
//
// Reports saved from Get help go where the user chose, so they're the
// user's files, not the app's, and aren't named here.
package userfiles

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// AppID is the app's ID: Fyne's preferences folder and the macOS
	// bundle ID and LaunchAgent label.
	AppID = "io.github.tucono.nebulagui"
	// DirName is the app's folder inside the user's config and cache
	// folders.
	DirName = "nebula-gui"
	// AutostartName is the XDG autostart file's name on Linux.
	AutostartName = DirName + ".desktop"

	// RunKey is the HKCU key whose values Windows runs at sign-in.
	RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	// ApprovedKey is where Task Manager's Startup apps page records that
	// the user turned an entry off, leaving the Run value in place.
	ApprovedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	// RunValue is the app's value under RunKey and ApprovedKey.
	RunValue = DirName
)

// Dir is this user's settings folder, <user config dir>/nebula-gui: saved
// peer names, masking names and the app log.
func Dir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, DirName), nil
}

// CacheDir is this user's cache folder, <user cache dir>/nebula-gui.
func CacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, DirName), nil
}

// Files are one user's nebula-gui files.
type Files struct {
	// Dirs are removed whole: the settings folder, Fyne's preferences
	// folder and the cache folder.
	Dirs []string
	// Autostart is the start-at-sign-in file, or empty on Windows, where
	// it's the RunValue registry value.
	Autostart string
}

// For gives a user's files on goos from their home folder, where the app
// puts them when that user runs it. A user who has moved their config
// folder elsewhere (XDG_CONFIG_HOME on Linux) has their files there
// instead, which can't be seen from outside their session.
func For(goos, home string) Files {
	f := relFor(goos)
	for i, d := range f.Dirs {
		f.Dirs[i] = join(goos, home, d)
	}
	if f.Autostart != "" {
		f.Autostart = join(goos, home, f.Autostart)
	}
	return f
}

// relFor gives a user's files on goos relative to their home folder.
func relFor(goos string) Files {
	j := func(elem ...string) string { return join(goos, elem...) }
	switch goos {
	case "windows":
		return Files{Dirs: []string{
			j("AppData", "Roaming", DirName),
			j("AppData", "Roaming", "fyne", AppID),
		}}
	case "darwin":
		return Files{
			Dirs: []string{
				j("Library", "Application Support", DirName),
				j("Library", "Preferences", "fyne", AppID),
				j("Library", "Caches", DirName),
			},
			Autostart: j("Library", "LaunchAgents", AppID+".plist"),
		}
	default:
		return Files{
			Dirs: []string{
				j(".config", DirName),
				j(".config", "fyne", AppID),
				j(".cache", DirName),
			},
			Autostart: j(".config", "autostart", AutostartName),
		}
	}
}

// join joins path elements with goos's separator, so For gives Windows
// paths when run elsewhere (in tests).
func join(goos string, elem ...string) string {
	if goos != "windows" {
		return path.Join(elem...)
	}
	for i, e := range elem {
		elem[i] = strings.ReplaceAll(e, `\`, "/")
	}
	return strings.ReplaceAll(path.Join(elem...), "/", `\`)
}
