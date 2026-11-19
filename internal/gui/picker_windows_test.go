//go:build windows

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"slices"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsFilter(t *testing.T) {
	cases := []struct {
		name string
		exts []string
		want []string
	}{
		{"", configExtensions, []string{"Config files (*.yml;*.yaml;*.conf;*.txt)", "*.yml;*.yaml;*.conf;*.txt", "All files (*.*)", "*.*"}},
		{"Markdown files", []string{".md"}, []string{"Markdown files (*.md)", "*.md", "All files (*.*)", "*.*"}},
		{"", nil, []string{"All files (*.*)", "*.*"}},
	}
	for _, c := range cases {
		if got := windowsFilter(c.name, c.exts); !slices.Equal(got, c.want) {
			t.Errorf("windowsFilter(%q, %q) = %q, want %q", c.name, c.exts, got, c.want)
		}
	}
}

func TestUTF16Multi(t *testing.T) {
	got := utf16Multi([]string{"a", "bc"})
	want := []uint16{'a', 0, 'b', 'c', 0, 0}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if s := windows.UTF16ToString(got); s != "a" {
		t.Fatalf("first entry %q", s)
	}
}

// OPENFILENAMEW is 152 bytes on 64-bit Windows; a wrong layout makes the
// dialog fail with CDERR_STRUCTSIZE.
func TestOpenFileNameSize(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && unsafe.Sizeof(openFileName{}) != 152 {
		t.Fatalf("size %d, want 152", unsafe.Sizeof(openFileName{}))
	}
}
