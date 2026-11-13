// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"slices"
	"testing"
)

// fakeNative records requests and answers with the next result.
type fakeNative struct {
	reqs    []openRequest
	paths   []string
	errs    []error
	parents []string
}

func (f *fakeNative) open(parent string, req openRequest) (string, error) {
	f.reqs = append(f.reqs, req)
	f.parents = append(f.parents, parent)
	i := len(f.reqs) - 1
	var path string
	var err error
	if i < len(f.paths) {
		path = f.paths[i]
	}
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return path, err
}

func TestNativePickerImports(t *testing.T) {
	svc := newFakeService()
	files := fakeFS{}
	for k, v := range labFiles {
		files[k] = v
	}
	files["/home/alice/Downloads/config.yml"] = "pki:\n  ca: ca.crt\n  cert: laptop.crt\n  key: laptop.key\n"
	m, poll := importWindow(t, svc, files)
	poll()
	nat := &fakeNative{paths: []string{abs("/home/alice/Downloads/config.yml")}}
	m.nativeOpen = nat.open

	m.importConfig()
	if len(nat.reqs) != 1 || !slices.Equal(nat.reqs[0].Extensions, configExtensions) {
		t.Fatalf("requests %+v", nat.reqs)
	}
	if m.picker != nil {
		t.Fatal("Fyne's picker opened too")
	}
	if len(svc.put) != 1 {
		t.Fatalf("config not imported: %v", svc.takeCalls())
	}
	if m.picking {
		t.Fatal("still marked as picking")
	}
}

func TestNativePickerCancelled(t *testing.T) {
	svc := newFakeService()
	m, poll := importWindow(t, svc, nil)
	poll()
	nat := &fakeNative{} // "" and no error: cancelled
	m.nativeOpen = nat.open
	m.importConfig()
	if m.picker != nil || len(svc.put) != 0 || m.w.Canvas().Overlays().Top() != nil {
		t.Fatal("cancel did something")
	}
}

func TestNativePickerFallsBack(t *testing.T) {
	m, poll := importWindow(t, newFakeService(), nil)
	poll()
	nat := &fakeNative{errs: []error{&errNoNativePicker{Err: errors.New("org.freedesktop.DBus.Error.ServiceUnknown")}}}
	m.nativeOpen = nat.open

	m.importConfig()
	if m.picker == nil || !m.noNative {
		t.Fatal("Fyne's picker not shown after the native one failed")
	}
	closeDialogs(m)
	m.picker = nil

	m.importConfig()
	if len(nat.reqs) != 1 || m.picker == nil {
		t.Fatalf("native picker tried again: %d requests", len(nat.reqs))
	}
	if !slices.Contains(msgs(m.log), "native file picker not available") {
		t.Fatalf("log %v", msgs(m.log))
	}
}

func TestChooseFileUsesNativePicker(t *testing.T) {
	svc := newFakeService()
	files := fakeFS{
		"/home/alice/Downloads/ca.crt":     caPEM,
		"/home/alice/Downloads/laptop.crt": certPEM,
		"/home/alice/Desktop/my.key":       keyPEM,
	}
	m, poll := importWindow(t, svc, files)
	poll()
	nat := &fakeNative{paths: []string{abs("/home/alice/Desktop/my.key")}}
	m.nativeOpen = nat.open

	m.startImport("config.yml", abs("/home/alice/Downloads"), []byte(pathsConfig), nil)
	choose := button(t, topDialog(t, m), "Choose the file…")
	if choose == nil {
		t.Fatal("no Choose the file… button")
	}
	choose.OnTapped()
	if len(nat.reqs) != 1 || nat.reqs[0].Extensions != nil || nat.reqs[0].Title != "Choose your key file" {
		t.Fatalf("request %+v", nat.reqs)
	}
	if len(svc.put) != 1 {
		t.Fatalf("not imported with the chosen key: %v", svc.takeCalls())
	}
}
