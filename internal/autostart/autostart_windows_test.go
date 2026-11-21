// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package autostart

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// testRunValue uses keys of its own under HKCU, removed afterwards, so the
// test never touches the real Run key.
func testRunValue(t *testing.T, exe string) *runValue {
	t.Helper()
	base := fmt.Sprintf(`Software\nebula-gui-test\%d-%s`, os.Getpid(), t.Name())
	r := &runValue{run: base + `\Run`, approved: base + `\Approved`, exe: exe}
	t.Cleanup(func() {
		for _, k := range []string{r.run, r.approved, base} {
			_ = registry.DeleteKey(registry.CURRENT_USER, k)
		}
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\nebula-gui-test`) // fails while others remain
	})
	return r
}

func enabled(t *testing.T, e Entry) bool {
	t.Helper()
	on, err := e.Enabled()
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	return on
}

func TestRunValueOnAndOff(t *testing.T) {
	r := testRunValue(t, `C:\Program Files\nebula-gui\nebula-gui.exe`)
	if enabled(t, r) {
		t.Fatal("enabled before it was set")
	}
	if err := r.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !enabled(t, r) {
		t.Fatal("not enabled after SetEnabled(true)")
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, r.run, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := k.GetStringValue("nebula-gui")
	_ = k.Close()
	if want := `"C:\Program Files\nebula-gui\nebula-gui.exe" --hidden`; err != nil || v != want {
		t.Fatalf("value %q, %v; want %q", v, err, want)
	}

	if err := r.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if enabled(t, r) {
		t.Fatal("still enabled after SetEnabled(false)")
	}
	if err := r.SetEnabled(false); err != nil {
		t.Fatalf("turning off twice: %v", err)
	}
}

func TestRunValueTurnedOffInTaskManager(t *testing.T) {
	r := testRunValue(t, `C:\nebula-gui.exe`)
	if err := r.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, r.approved, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	// Task Manager writes 12 bytes; an odd first byte means off.
	err = k.SetBinaryValue("nebula-gui", []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	_ = k.Close()
	if err != nil {
		t.Fatal(err)
	}
	if enabled(t, r) {
		t.Fatal("enabled although Task Manager turned it off")
	}
	if err := r.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !enabled(t, r) {
		t.Fatal("turning it on again didn't clear Task Manager's record")
	}
}
