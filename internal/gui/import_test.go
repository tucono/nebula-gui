// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

const pathsConfig = "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.key\nsshd:\n  enabled: true\n"

// importWindow returns a test window whose pki files come from a fake.
func importWindow(t *testing.T, svc *fakeService, files fakeFS) (*mainWindow, func()) {
	t.Helper()
	m, poll := testWindow(t, svc)
	m.w.Resize(fyne.NewSize(520, 420))
	f := fakeFS{}
	for k, v := range files {
		f[filepath.ToSlash(abs(k))] = v
	}
	m.readFile = f.read
	return m, poll
}

func topDialog(t *testing.T, m *mainWindow) fyne.CanvasObject {
	t.Helper()
	o := m.w.Canvas().Overlays().Top()
	if o == nil {
		t.Fatal("no dialog shown")
	}
	return o
}

func closeDialogs(m *mainWindow) {
	for m.w.Canvas().Overlays().Top() != nil {
		m.w.Canvas().Overlays().Remove(m.w.Canvas().Overlays().Top())
	}
}

var labFiles = fakeFS{
	"/home/alice/Downloads/ca.crt":     caPEM,
	"/home/alice/Downloads/laptop.crt": certPEM,
	"/home/alice/Downloads/laptop.key": keyPEM,
}

func TestImportFirstConfig(t *testing.T) {
	svc := newFakeService()
	svc.unconfig = true
	m, poll := importWindow(t, svc, labFiles)
	poll()
	if p := m.banner.Problem(); p == nil || p.ID != probNoConfig || button(t, m.w.Content(), "Import config…") == nil {
		t.Fatalf("empty state not shown: %+v", p)
	}

	m.startImport("config.yml", abs("/home/alice/Downloads"), []byte("pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.key\n"), nil)
	if len(svc.put) != 1 || !strings.Contains(svc.put[0], "BEGIN NEBULA X25519 PRIVATE KEY") {
		t.Fatalf("put %q", svc.put)
	}
	if label(t, topDialog(t, m), "You're ready to connect.") == nil {
		t.Fatal("no success message")
	}
	closeDialogs(m)
	poll()
	if m.banner.Problem() != nil || !m.header.Visible() {
		t.Fatalf("still unconfigured after import: %+v", m.banner.Problem())
	}

	got := strings.Join(msgs(m.log), ",")
	if !strings.Contains(got, "import started,import done") {
		t.Fatalf("log %s", got)
	}
	for _, e := range m.log.Recent(appLogKeep) {
		for _, v := range e.Attrs {
			if strings.Contains(v, "PRIVATE KEY") || strings.Contains(v, "/Downloads") {
				t.Fatalf("log entry %+v has config contents or a path", e)
			}
		}
	}
}

func TestImportSSHDFix(t *testing.T) {
	svc := newFakeService()
	svc.putErrs = []error{&api.Error{Code: api.CodeInvalidConfig, Reason: api.ReasonSSHDEnabled, Message: "sshd.enabled must be false"}}
	m, poll := importWindow(t, svc, labFiles)
	poll()

	m.startImport("config.yml", abs("/home/alice/Downloads"), []byte(pathsConfig), nil)
	d := topDialog(t, m)
	b := button(t, d, "Turn it off and import")
	if b == nil {
		t.Fatal("no fix button")
	}
	test.Tap(b)
	if len(svc.put) != 1 || !strings.Contains(svc.put[0], "enabled: false") {
		t.Fatalf("put %q", svc.put)
	}
	if label(t, topDialog(t, m), "You're ready to connect.") == nil {
		t.Fatal("no success message after the fix")
	}
}

func TestImportMissingFile(t *testing.T) {
	svc := newFakeService()
	m, poll := importWindow(t, svc, fakeFS{
		"/home/alice/Downloads/ca.crt":     caPEM,
		"/home/alice/Downloads/laptop.crt": certPEM,
	})
	poll()
	m.startImport("config.yml", abs("/home/alice/Downloads"), []byte(pathsConfig), nil)
	d := topDialog(t, m)
	if label(t, d, "A file is missing: laptop.key") == nil || button(t, d, "Choose the file…") == nil {
		t.Fatal("missing-file dialog not shown")
	}
	if len(svc.put) != 0 {
		t.Fatal("sent a config with a missing file")
	}
}

func TestImportNotAConfig(t *testing.T) {
	svc := newFakeService()
	m, poll := importWindow(t, svc, nil)
	poll()
	m.startImport("notes.txt", abs("/home/alice"), []byte("Buy milk\n"), nil)
	d := topDialog(t, m)
	if label(t, d, "That doesn't look like a Nebula config file") == nil || button(t, d, "Choose another file…") == nil {
		t.Fatal("not-a-config dialog not shown")
	}
}

func TestImportWhileConnected(t *testing.T) {
	svc := newFakeService()
	m, poll := importWindow(t, svc, labFiles)
	poll()
	test.Tap(m.button) // connect
	if m.headline.Text != "Connected" {
		t.Fatalf("headline %q", m.headline.Text)
	}

	m.startImport("config.yml", abs("/home/alice/Downloads"), []byte("pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.key\n"), nil)
	closeDialogs(m)
	poll()
	p := m.banner.Problem()
	if p == nil || p.ID != probNewConfigSaved || !m.header.Visible() {
		t.Fatalf("banner %+v", p)
	}
	svc.takeCalls()
	test.Tap(button(t, m.w.Content(), "Reconnect to use it"))
	if c := svc.takeCalls(); !equal(c, []string{"disconnect", "connect"}) {
		t.Fatalf("calls %v", c)
	}
	if m.banner.Problem() != nil || m.headline.Text != "Connected" {
		t.Fatalf("after reconnect: banner %+v, headline %q", m.banner.Problem(), m.headline.Text)
	}
}

func TestRemoveConfig(t *testing.T) {
	svc := newFakeService()
	m, poll := importWindow(t, svc, nil)
	poll()
	test.Tap(m.removeB)
	d := topDialog(t, m)
	yes, _ := find(t, d, func(b *widget.Button) bool { return b.Text == "Yes" })
	if yes == nil {
		t.Fatal("no confirmation")
	}
	test.Tap(yes)
	if c := svc.takeCalls(); c[len(c)-1] != "delete" {
		t.Fatalf("calls %v", c)
	}
	if p := m.banner.Problem(); p == nil || p.ID != probNoConfig || m.header.Visible() {
		t.Fatalf("after remove: banner %+v", p)
	}
}
