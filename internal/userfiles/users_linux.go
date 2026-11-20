// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"bytes"
	"os"
	"os/exec"
)

// users asks getent, which also sees accounts from a directory service
// (LDAP, SSSD), and falls back to /etc/passwd.
func users() ([]User, error) {
	out, err := exec.Command("getent", "passwd").Output()
	if err == nil {
		return parsePasswd(bytes.NewReader(out)), nil
	}
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parsePasswd(f), nil
}
