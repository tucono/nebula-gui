// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/tucono/nebula-gui/internal/api"
)

// configExtensions are the files the import picker offers. A config the
// user was given has one of these; files without an extension aren't
// offered.
var configExtensions = []string{".yml", ".yaml", ".conf", ".txt"}

// importFlow is one import, from the picked config to the service's
// answer. It lives across the dialogs that ask for a missing file or offer
// a fix, so each fix adds to what the user already chose.
type importFlow struct {
	m    *mainWindow
	name string // the config's file name, for messages and the GUI log
	raw  []byte
	opts inlineOptions
	// names are the pki file names from the last inlining, for messages
	// about the service's answer.
	names map[string]string
}

// importConfig asks for a config file and imports it.
func (m *mainWindow) importConfig() {
	if m.importing {
		return
	}
	m.pickFile(openRequest{Title: "Import config", Extensions: configExtensions}, func(path string) {
		raw, err := m.readFile(path)
		m.startImport(fileName(path), filepath.Dir(path), raw, err)
	})
}

// startImport imports the config raw, read from the file name in dir.
func (m *mainWindow) startImport(name, dir string, raw []byte, readErr error) {
	home, _ := os.UserHomeDir()
	f := &importFlow{
		m:    m,
		name: name,
		raw:  raw,
		opts: inlineOptions{
			BaseDir: dir,
			Home:    home,
			Picked:  map[string]string{},
			Read:    m.readFile,
		},
	}
	m.log.Add("import started", "file", name)
	switch {
	case readErr != nil:
		f.fail(&errFileUnreadable{Path: name, Err: readErr})
	case len(raw) > maxFileBytes:
		f.fail(&errTooLarge{})
	default:
		f.run()
	}
}

// run inlines the pki files and sends the result. The files and the
// service call are handled off the UI goroutine.
func (f *importFlow) run() {
	m := f.m
	m.importing = true
	m.render()
	m.spawn(func() {
		in, err := inlinePKI(f.raw, f.opts)
		var resp *api.ConfigPutResponse
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
			resp, err = m.c.PutConfig(ctx, in.YAML)
			cancel()
		}
		fyne.Do(func() {
			m.importing = false
			if in != nil {
				f.names = in.Names
			}
			if err != nil {
				f.fail(err)
			} else {
				f.done(resp)
			}
			m.render()
		})
	})
}

func (f *importFlow) done(resp *api.ConfigPutResponse) {
	m := f.m
	m.log.Add("import done", "file", f.name, "restart_required", strconv.FormatBool(resp.RestartRequired))
	msg := "You're ready to connect."
	if resp.RestartRequired {
		m.in.NewConfig = true
		msg = "You're still connected with the old config. Click Reconnect to use the new one."
	}
	m.in.Action = nil // a problem from before the import is out of date
	m.poller.Poke()   // configured, cert and overlay IP may have changed
	dialog.ShowInformation("Config imported", msg, m.w)
}

// fail shows the problem for err in a dialog, with fixes that continue
// this import.
func (f *importFlow) fail(err error) {
	m := f.m
	p := f.problem(err)
	m.log.Add("import failed", "file", f.name, "problem", string(p.ID), "error", logError(err))
	showProblemDialog(m.w, p, problemHandlers{
		OnFix:    f.fix,
		Supports: importFixes,
		Versions: m.versionsLine,
	})
}

// problem picks the catalogue entry for an import error.
func (f *importFlow) problem(err error) *problem {
	if p := inlineProblem(err, f.name); p != nil {
		return p
	}
	return problemFor(err, problemContext{
		Op:         opImport,
		ConfigName: f.name,
		KeyName:    f.names["key"],
		CertName:   f.names["cert"],
		GOOS:       f.m.in.GOOS,
	})
}

// importFixes are the fixes an import problem dialog offers. Try again
// (the service isn't running) runs the import again.
func importFixes(k fixKind) bool {
	switch k {
	case fixChooseConfig, fixChooseFile, fixDisableSSHD, fixRetry:
		return true
	default:
		return false
	}
}

func (f *importFlow) fix(a fixAction) {
	m := f.m
	switch a.Kind {
	case fixChooseConfig:
		m.importConfig()
	case fixChooseFile:
		f.chooseFile(a.PKIKey)
	case fixDisableSSHD:
		m.log.Add("import fix", "file", f.name, "fix", "disable_sshd")
		f.opts.DisableSSHD = true
		f.run()
	case fixRetry:
		f.run()
	}
}

// chooseFile asks for the file for pki entry key, starting in the home
// folder with no filter, and continues the import with it.
func (f *importFlow) chooseFile(key string) {
	m := f.m
	title := "Choose your certificate file"
	if key == "key" {
		title = "Choose your key file"
	}
	m.pickFile(openRequest{Title: title, StartDir: f.opts.Home}, func(path string) {
		m.log.Add("import file chosen", "file", f.name, "for", "pki."+key, "chosen", fileName(path))
		f.opts.Picked[key] = path
		f.run()
	})
}

// inlineProblem returns the catalogue entry for an error from inlinePKI or
// from reading the config, or nil for any other error.
func inlineProblem(err error, configName string) *problem {
	var (
		notConfig  *errNotConfig
		incomplete *errPKIIncomplete
		missing    *errFileMissing
		unreadable *errFileUnreadable
		notPEM     *errNotPEM
		tooLarge   *errTooLarge
		p          *problem
	)
	switch {
	case errors.As(err, &notConfig):
		p = notAConfig(orDefault(configName, "a file"))
	case errors.As(err, &incomplete):
		p = pkiMissing()
	case errors.As(err, &missing):
		p = fileMissing(missing.Key, baseName(missing.Path))
	case errors.As(err, &unreadable):
		if unreadable.Key == "" { // the config file itself
			p = fileUnreadable("", baseName(unreadable.Path))
			p.Fix = &fixAction{Label: "Choose another file…", Kind: fixChooseConfig}
		} else {
			p = fileUnreadable(unreadable.Key, baseName(unreadable.Path))
		}
	case errors.As(err, &notPEM):
		p = fileNotPEM(notPEM.Key, baseName(notPEM.Path))
	case errors.As(err, &tooLarge):
		if tooLarge.Key == "" {
			p = configTooLarge()
		} else {
			// A pki file over 1 MiB can't be a certificate or key.
			p = fileNotPEM(tooLarge.Key, baseName(tooLarge.Path))
		}
	default:
		return nil
	}
	p.Details = joinDetails(p.Details, err.Error())
	return p
}

// logError is err for the GUI log: a service error as "code / reason:
// message", an inline error with file names instead of full paths.
func logError(err error) string {
	var (
		missing    *errFileMissing
		unreadable *errFileUnreadable
		notPEM     *errNotPEM
		tooLarge   *errTooLarge
	)
	switch {
	case errors.As(err, &missing):
		c := *missing
		c.Path = baseName(c.Path)
		return c.Error()
	case errors.As(err, &unreadable):
		c := *unreadable
		c.Path = baseName(c.Path)
		var pe *os.PathError
		if errors.As(c.Err, &pe) {
			c.Err = pe.Err
		}
		return c.Error()
	case errors.As(err, &notPEM):
		c := *notPEM
		c.Path = baseName(c.Path)
		return c.Error()
	case errors.As(err, &tooLarge):
		c := *tooLarge
		c.Path = baseName(c.Path)
		return c.Error()
	default:
		return errorDetails(err)
	}
}
