// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// scratchKey makes an empty key under HKCU\Software for one test and
// removes it afterwards.
func scratchKey(t *testing.T) (registry.Key, string) {
	t.Helper()
	path := fmt.Sprintf(`Software\nebula-gui-test-%d`, time.Now().UnixNano())
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = k.Close()
		deleteTree(t, registry.CURRENT_USER, path)
	})
	return k, path
}

func deleteTree(t *testing.T, parent registry.Key, path string) {
	t.Helper()
	k, err := registry.OpenKey(parent, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	subs, _ := k.ReadSubKeyNames(-1)
	_ = k.Close()
	for _, s := range subs {
		deleteTree(t, parent, path+`\`+s)
	}
	if err := registry.DeleteKey(parent, path); err != nil {
		t.Errorf("delete %s: %v", path, err)
	}
}

func createKey(t *testing.T, parent registry.Key, path string) registry.Key {
	t.Helper()
	k, _, err := registry.CreateKey(parent, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k.Close() })
	return k
}

func TestProfilesIn(t *testing.T) {
	k, _ := scratchKey(t)
	profile := func(sid, folder string) {
		pk := createKey(t, k, sid)
		if folder != "" {
			if err := pk.SetExpandStringValue("ProfileImagePath", folder); err != nil {
				t.Fatal(err)
			}
		}
	}
	profile("S-1-5-18", `%systemroot%\system32\config\systemprofile`)
	profile("S-1-5-21-1-2-3-1001", `%SystemDrive%\Users\alice`)
	profile("S-1-5-21-1-2-3-1001.bak", `%SystemDrive%\Users\alice.old`)
	profile("S-1-5-21-1-2-3-1002", "")
	profile("S-1-5-21-1-2-3-1003", `D:\Profiles\bob`)

	got, err := profilesIn(k)
	if err != nil {
		t.Fatal(err)
	}
	// The SIDs are made up, so the names fall back to the folders.
	want := []User{
		{Name: "alice", Home: os.Getenv("SystemDrive") + `\Users\alice`, SID: "S-1-5-21-1-2-3-1001"},
		{Name: "bob", Home: `D:\Profiles\bob`, SID: "S-1-5-21-1-2-3-1003"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("profilesIn = %+v, want %+v", got, want)
	}
}

// fakeRunValues gives a hive at k the app's start-at-sign-in values and
// another app's beside them.
func fakeRunValues(t *testing.T, k registry.Key) {
	t.Helper()
	run := createKey(t, k, RunKey)
	approved := createKey(t, k, ApprovedKey)
	for _, name := range []string{RunValue, "other-app"} {
		if err := run.SetStringValue(name, `"C:\x.exe"`); err != nil {
			t.Fatal(err)
		}
		if err := approved.SetBinaryValue(name, []byte{3, 0, 0, 0}); err != nil {
			t.Fatal(err)
		}
	}
}

// checkRunValues checks the app's values are gone and the other app's
// are kept.
func checkRunValues(t *testing.T, k registry.Key) {
	t.Helper()
	for _, path := range []string{RunKey, ApprovedKey} {
		sk, err := registry.OpenKey(k, path, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		names, err := sk.ReadValueNames(-1)
		_ = sk.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(names, []string{"other-app"}) {
			t.Errorf("%s holds %q, want only other-app", path, names)
		}
	}
}

func TestRemoveRunValues(t *testing.T) {
	k, _ := scratchKey(t)
	fakeRunValues(t, k)
	removed, err := removeRunValues(k, "HIVE")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`HIVE\` + RunKey + `\` + RunValue, `HIVE\` + ApprovedKey + `\` + RunValue}
	if !slices.Equal(removed, want) {
		t.Errorf("removed %q, want %q", removed, want)
	}
	checkRunValues(t, k)

	// Again, with nothing left to remove, and with no keys at all.
	if removed, err := removeRunValues(k, "HIVE"); err != nil || len(removed) != 0 {
		t.Errorf("second run = %q, %v", removed, err)
	}
	empty, _ := scratchKey(t)
	if removed, err := removeRunValues(empty, "HIVE"); err != nil || len(removed) != 0 {
		t.Errorf("empty hive = %q, %v", removed, err)
	}
}

func TestRemoveAutostartValuesNoHive(t *testing.T) {
	u := User{Name: "dev", Home: t.TempDir(), SID: "S-1-5-21-0-0-0-99998"}
	if removed, err := removeAutostartValues(u); err != nil || len(removed) != 0 {
		t.Errorf("a profile without NTUSER.DAT = %q, %v", removed, err)
	}
}

// TestRemoveAutostartValuesFromHiveFile saves a fake hive to NTUSER.DAT
// in a fake profile, as for someone who isn't signed in, and removes the
// values from it. Saving and loading hives needs an administrator, which
// the CI runner is.
func TestRemoveAutostartValuesFromHiveFile(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("loading a hive needs an administrator")
	}
	if err := enableHivePrivileges(); err != nil {
		t.Fatal(err)
	}
	src, _ := scratchKey(t)
	fakeRunValues(t, src)
	home := t.TempDir()
	hive := filepath.Join(home, "NTUSER.DAT")
	if err := saveKey(src, hive); err != nil {
		t.Fatal(err)
	}

	u := User{Name: "bob", Home: home, SID: "S-1-5-21-0-0-0-99999"}
	removed, err := removeAutostartValues(u)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("removed %q, want both values", removed)
	}
	if _, err := registry.OpenKey(registry.USERS, hiveKeyPrefix+u.SID, registry.QUERY_VALUE); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the hive is still loaded: %v", err)
	}
	// The change was written to the file.
	err = withHive(hive, hiveKeyPrefix+"check", func(k registry.Key) error {
		checkRunValues(t, k)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func saveKey(k registry.Key, file string) error {
	p := advapi32.NewProc("RegSaveKeyW")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	if err := p.Find(); err != nil {
		return err
	}
	r, _, _ := p.Call(uintptr(k), uintptr(unsafe.Pointer(f)), 0) //nolint:gosec // as for regLoadKey
	if err := regErr(r); err != nil {
		return fmt.Errorf("save %s: %w", file, err)
	}
	return nil
}
