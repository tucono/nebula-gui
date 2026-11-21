//go:build darwin && !ios

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchAgentOnAndOff(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	exe := "/Applications/Tom & Jerry/nebula-gui.app/Contents/MacOS/nebula-gui"
	e, err := New(exe)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "io.github.tucono.nebulagui.plist")

	if on, err := e.Enabled(); on || err != nil {
		t.Fatalf("before: %v, %v", on, err)
	}
	if err := e.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if on, err := e.Enabled(); !on || err != nil {
		t.Fatalf("after on: %v, %v", on, err)
	}
	b, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	// Well-formed, with the path escaped and read back as written.
	var strs []string
	d := xml.NewDecoder(bytes.NewReader(b))
	inString := false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("not well-formed: %v\n%s", err, b)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			inString = tok.Name.Local == "string"
		case xml.CharData:
			if inString {
				strs = append(strs, string(tok))
			}
		case xml.EndElement:
			inString = false
		}
	}
	want := []string{"io.github.tucono.nebulagui", exe, "--hidden", "Aqua", "Interactive"}
	if strings.Join(strs, "|") != strings.Join(want, "|") {
		t.Fatalf("strings %q, want %q", strs, want)
	}
	if !bytes.Contains(b, []byte("<key>RunAtLoad</key>\n\t<true/>")) {
		t.Fatalf("no RunAtLoad:\n%s", b)
	}

	if err := e.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if on, err := e.Enabled(); on || err != nil {
		t.Fatalf("after off: %v, %v", on, err)
	}
	if err := e.SetEnabled(false); err != nil {
		t.Fatalf("turning off twice: %v", err)
	}
}
