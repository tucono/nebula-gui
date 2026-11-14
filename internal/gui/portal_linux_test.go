// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"bytes"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestRequestPath(t *testing.T) {
	got := requestPath(":1.42", "nebulagui_ab")
	if got != "/org/freedesktop/portal/desktop/request/1_42/nebulagui_ab" {
		t.Fatalf("got %s", got)
	}
}

func TestPathFromURI(t *testing.T) {
	ok := map[string]string{
		"file:///home/alice/config.yml":                 "/home/alice/config.yml",
		"file:///home/alice/My%20Files/a+b.yml":         "/home/alice/My Files/a+b.yml",
		"file://localhost/home/alice/%C3%A9t%C3%A9.yml": "/home/alice/été.yml",
	}
	for in, want := range ok {
		if got, err := pathFromURI(in); err != nil || got != want {
			t.Errorf("pathFromURI(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"sftp://host/config.yml", "file://otherhost/config.yml", "%zz"} {
		if _, err := pathFromURI(in); err == nil {
			t.Errorf("pathFromURI(%q) accepted", in)
		}
	}
}

func TestParseResponse(t *testing.T) {
	results := map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///home/alice/config.yml"})}
	if p, err := parseResponse([]any{uint32(0), results}); err != nil || p != "/home/alice/config.yml" {
		t.Fatalf("success: %q, %v", p, err)
	}
	for _, status := range []uint32{1, 2} {
		if p, err := parseResponse([]any{status, map[string]dbus.Variant{}}); err != nil || p != "" {
			t.Fatalf("status %d: %q, %v", status, p, err)
		}
	}
	if p, err := parseResponse([]any{uint32(0), map[string]dbus.Variant{}}); err != nil || p != "" {
		t.Fatalf("no uris: %q, %v", p, err)
	}
	if _, err := parseResponse([]any{"x"}); err == nil {
		t.Fatal("bad body accepted")
	}
}

func TestPortalOptions(t *testing.T) {
	o := portalOptions(openRequest{Extensions: []string{".yml"}, StartDir: "/home/alice"}, "tok")
	if o["handle_token"].Value() != "tok" || o["modal"].Value() != true {
		t.Fatalf("options %v", o)
	}
	if dir := o["current_folder"].Value().([]byte); !bytes.Equal(dir, []byte("/home/alice\x00")) {
		t.Fatalf("current_folder %q", dir)
	}
	fs := o["filters"].Value().([]portalFilter)
	if len(fs) != 1 || len(fs[0].Rules) != 2 || fs[0].Rules[0].Pattern != "*.yml" || fs[0].Rules[1].Pattern != "*.YML" {
		t.Fatalf("filters %+v", fs)
	}
	if sig := o["filters"].Signature().String(); sig != "a(sa(us))" {
		t.Fatalf("filters signature %s", sig)
	}
	if sig := o["current_filter"].Signature().String(); sig != "(sa(us))" {
		t.Fatalf("current_filter signature %s", sig)
	}

	o = portalOptions(openRequest{}, "tok")
	if _, ok := o["filters"]; ok {
		t.Fatal("filter without extensions")
	}
	if _, ok := o["current_folder"]; ok {
		t.Fatal("current_folder without StartDir")
	}
	if _, ok := o["current_name"]; ok {
		t.Fatal("current_name when opening")
	}
	if fs[0].Name != "Config files (.yml)" {
		t.Fatalf("filter name %q", fs[0].Name)
	}

	o = portalOptions(openRequest{Save: true, FileName: "nebula-report-2026-11-14.md", FilterName: "Markdown files", Extensions: []string{".md"}}, "tok")
	if o["current_name"].Value() != "nebula-report-2026-11-14.md" {
		t.Fatalf("current_name %v", o["current_name"])
	}
	if fs := o["filters"].Value().([]portalFilter); fs[0].Name != "Markdown files (.md)" {
		t.Fatalf("filter name %q", fs[0].Name)
	}
}
