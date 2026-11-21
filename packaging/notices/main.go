// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

// Command notices writes THIRD_PARTY_NOTICES: the license texts of Go and
// of every module built into nebula-gui and nebula-guid, which their
// licenses require copies to carry. It lists the modules for the
// platform in GOOS and GOARCH, so run it with the same environment as
// the build. The packages install the file next to the programs.
//
//	go run ./packaging/notices [-out dist/THIRD_PARTY_NOTICES.txt] [packages]
//
// The packages default to ./cmd/.... It fails when a module has no
// license file at its root, so a new dependency can't ship unnoticed.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// header starts the file.
const header = `nebula-gui third-party notices
==============================

nebula-gui is an independent project. It is not affiliated with or
endorsed by Slack Technologies or Defined Networking. Nebula is
copyright Slack Technologies, Inc. and is MIT licensed.

nebula-gui includes the software below. Each part's license follows
its name.
`

// wintun goes after the header on Windows, which also ships Wintun.
const wintun = `
It also includes Wintun, whose license is in
dist\windows\wintun\LICENSE.txt in the install folder.
`

// licenseFile matches the names of the files copied from a module's root.
var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice|patents)([.-].*)?$`)

type module struct {
	Path    string
	Version string
	Dir     string
	Main    bool
	Replace *module
}

type pkg struct {
	ImportPath string
	Module     *module
}

func main() {
	out := flag.String("out", "dist/THIRD_PARTY_NOTICES.txt", "file to write")
	flag.Parse()
	pkgs := flag.Args()
	if len(pkgs) == 0 {
		pkgs = []string{"./cmd/..."}
	}
	text, err := notices(pkgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
	// The packages set the installed file's mode.
	if err := os.WriteFile(*out, []byte(text), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "notices:", err)
		os.Exit(1)
	}
}

// notices builds the file's text for the packages and their dependencies.
func notices(pkgs []string) (string, error) {
	mods, err := modules(pkgs)
	if err != nil {
		return "", err
	}
	goroot, err := goEnv("GOROOT")
	if err != nil {
		return "", err
	}
	goVersion, err := goEnv("GOVERSION")
	if err != nil {
		return "", err
	}
	goos, err := goEnv("GOOS")
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(header)
	if goos == "windows" {
		b.WriteString(wintun)
	}
	var missing []string
	add := func(name, dir string) {
		files, err := licenseFiles(dir)
		if err != nil || len(files) == 0 {
			missing = append(missing, name)
			return
		}
		for _, f := range files {
			data, err := os.ReadFile(filepath.Join(dir, f)) //nolint:gosec // a license file in the module cache
			if err != nil {
				missing = append(missing, name)
				return
			}
			fmt.Fprintf(&b, "\n\n%s\n%s (%s)\n%s\n\n", strings.Repeat("=", 72), name, f, strings.Repeat("-", 72))
			b.Write(bytes.TrimSpace(normalize(data)))
			b.WriteString("\n")
		}
	}
	add("Go "+goVersion+" (standard library and runtime)", goroot)
	for _, m := range mods {
		add(m.Path+" "+m.Version, m.Dir)
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("no license file found for: %s", strings.Join(missing, ", "))
	}
	return b.String(), nil
}

// modules lists the modules other than this one that provide the
// packages or their dependencies, sorted by path.
func modules(pkgs []string) ([]module, error) {
	cmd := exec.Command("go", append([]string{"list", "-deps", "-json=ImportPath,Module"}, pkgs...)...) //nolint:gosec // package patterns from the Makefile
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	seen := map[string]module{}
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var p pkg
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("go list output: %w", err)
		}
		m := p.Module
		if m == nil || m.Main {
			continue // the standard library, or this module
		}
		if m.Replace != nil {
			r := *m.Replace
			r.Path = m.Path + " => " + r.Path
			m = &r
		}
		if m.Dir == "" {
			return nil, fmt.Errorf("module %s isn't downloaded (run go mod download)", m.Path)
		}
		seen[m.Path] = *m
	}
	mods := make([]module, 0, len(seen))
	for _, m := range seen {
		mods = append(mods, m)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	return mods, nil
}

// licenseFiles lists the license files in dir, sorted.
func licenseFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.Type().IsRegular() && licenseFile.MatchString(e.Name()) {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

// normalize uses \n line endings, whatever the file had.
func normalize(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

func goEnv(name string) (string, error) {
	out, err := exec.Command("go", "env", name).Output() //nolint:gosec // names are constants
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}
