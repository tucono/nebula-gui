// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// User is a person's account on this computer, with their home folder.
type User struct {
	Name string
	Home string
}

// Users lists the people with accounts on this computer: on Linux the
// accounts with a UID of 1000 or more, on macOS the folders in /Users,
// and on Windows the profiles of real (not system) accounts. Service
// accounts are left out, since they don't run the app.
func Users() ([]User, error) { return users() }

// firstUID and nobodyUID bound the accounts people sign in with on Linux,
// as useradd and adduser number them.
const (
	firstUID  = 1000
	nobodyUID = 65534
)

// parsePasswd reads passwd(5) lines and returns the people's accounts:
// UIDs from firstUID, without nobody and without an absolute home folder.
func parsePasswd(r io.Reader) []User {
	var out []User
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) < 7 || f[0] == "" || strings.HasPrefix(f[0], "#") {
			continue
		}
		uid, err := strconv.Atoi(f[2])
		if err != nil || uid < firstUID || uid == nobodyUID {
			continue
		}
		home := f[5]
		if !filepath.IsAbs(home) || filepath.Clean(home) == "/" {
			continue
		}
		out = append(out, User{Name: f[0], Home: home})
	}
	return out
}

// homesIn lists the people's folders in dir, as /Users holds them on
// macOS: not Shared, and not hidden ones.
func homesIn(dir string) ([]User, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []User
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || name == "Shared" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, User{Name: name, Home: filepath.Join(dir, name)})
	}
	return out, nil
}
