//go:build linux && !android

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// desktopFile is ~/.config/autostart/nebula-gui.desktop, which desktops
// following the XDG autostart spec run at sign-in.
type desktopFile struct {
	path string
	exe  string
	// reload tells the desktop the file changed; nil in tests.
	reload func()
}

func newEntry(exe string) (Entry, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &desktopFile{
		path:   filepath.Join(dir, "autostart", "nebula-gui.desktop"),
		exe:    exe,
		reload: reloadUserManager,
	}, nil
}

// Enabled reports whether the file exists and isn't turned off inside, as
// some desktops' startup settings do instead of deleting it.
func (d *desktopFile) Enabled() (bool, error) {
	b, err := os.ReadFile(d.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !turnedOff(b), nil
}

func (d *desktopFile) SetEnabled(on bool) error {
	err := d.write(on)
	if err == nil && d.reload != nil {
		d.reload()
	}
	return err
}

func (d *desktopFile) write(on bool) error {
	if !on {
		return removeFile(d.path)
	}
	exec, err := execArg(d.exe)
	if err != nil {
		return err
	}
	return writeFile(d.path, []byte(fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=nebula-gui
Comment=Starts nebula-gui in the tray
Exec=%s %s
Terminal=false
`, exec, HiddenFlag)))
}

// reloadUserManager has systemd's user manager read the autostart files
// again. Desktops that start them through systemd (COSMIC, GNOME, KDE)
// turn each file into a unit only when the manager starts or reloads, and
// the manager often outlives signing out, so a new file would be missed
// and a removed one still run. It's done in the background; without
// systemd there's nothing to do.
func reloadUserManager() {
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "systemctl", "--user", "daemon-reload").Run()
	}()
}

// turnedOff reports whether a desktop entry says not to run it: Hidden=true
// (the spec's way) or X-GNOME-Autostart-enabled=false.
func turnedOff(b []byte) bool {
	s := bufio.NewScanner(bytes.NewReader(b))
	inEntry := false
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !inEntry || !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Hidden":
			if strings.TrimSpace(val) == "true" {
				return true
			}
		case "X-GNOME-Autostart-enabled":
			if strings.TrimSpace(val) == "false" {
				return true
			}
		}
	}
	return false
}

// execArg writes a path as one argument of a desktop entry's Exec key. A
// path with characters the spec reserves is quoted, with ", `, $ and \
// escaped inside the quotes; the key's value is a string, so each \ is
// then doubled again. % is a field code, so it's doubled.
func execArg(p string) (string, error) {
	if strings.ContainsAny(p, "\n\r") {
		return "", fmt.Errorf("the app's path has a line break: %q", p)
	}
	p = strings.ReplaceAll(p, "%", "%%")
	if !strings.ContainsAny(p, " \t\"'\\><~|&;$*?#()`") {
		return p, nil
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range p {
		switch r {
		case '\\':
			b.WriteString(`\\\\`)
		case '"', '`', '$':
			b.WriteString(`\\`)
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}
