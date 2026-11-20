//go:build darwin && !ios

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tucono/nebula-gui/internal/userfiles"
)

// agentLabel names the LaunchAgent and its file.
const agentLabel = userfiles.AppID

// launchAgent is ~/Library/LaunchAgents/<label>.plist, which launchd runs
// when the user signs in. It isn't loaded now; the app is already running.
type launchAgent struct {
	path string
	exe  string
}

func newEntry(exe string) (Entry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &launchAgent{path: filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"), exe: exe}, nil
}

func (l *launchAgent) Enabled() (bool, error) {
	_, err := os.Stat(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (l *launchAgent) SetEnabled(on bool) error {
	if !on {
		return removeFile(l.path)
	}
	var exe bytes.Buffer
	if err := xml.EscapeText(&exe, []byte(l.exe)); err != nil {
		return err
	}
	return writeFile(l.path, []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`, agentLabel, exe.String(), HiddenFlag)))
}
