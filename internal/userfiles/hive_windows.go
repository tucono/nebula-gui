// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package userfiles

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	advapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKey   = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKey = advapi32.NewProc("RegUnLoadKeyW")
	// AdjustTokenPrivileges is called directly, since x/sys/windows drops
	// the last error it sets when it succeeds; see enablePrivileges.
	procAdjustTokenPrivileges = advapi32.NewProc("AdjustTokenPrivileges")
)

// withHive loads the registry hive in file under HKEY_USERS\name, runs fn
// on it and unloads it. Loading needs the backup and restore privileges,
// which an administrator holds but has to turn on.
func withHive(file, name string, fn func(registry.Key) error) (err error) {
	if err := enableHivePrivileges(); err != nil {
		return err
	}
	if err := regLoadKey(name, file); err != nil {
		return fmt.Errorf("load %s: %w", file, err)
	}
	defer func() {
		if uerr := regUnLoadKey(name); uerr != nil {
			err = errors.Join(err, fmt.Errorf("unload %s from HKEY_USERS\\%s: %w", file, name, uerr))
		}
	}()
	// Every handle into the hive has to be closed before it unloads.
	k, err := registry.OpenKey(registry.USERS, name, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	err = fn(k)
	_ = k.Close()
	return err
}

// The registry functions below return their error code rather than
// setting the last error. Each converts its pointers to uintptr in the
// Call expression itself, the documented use of unsafe.Pointer with
// syscalls, so what they point to stays alive for the call.

func regLoadKey(name, file string) error {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return err
	}
	if err := procRegLoadKey.Find(); err != nil {
		return err
	}
	r, _, _ := procRegLoadKey.Call(uintptr(registry.USERS),
		uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(f))) //nolint:gosec // see above
	return regErr(r)
}

func regUnLoadKey(name string) error {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	if err := procRegUnLoadKey.Find(); err != nil {
		return err
	}
	r, _, _ := procRegUnLoadKey.Call(uintptr(registry.USERS), uintptr(unsafe.Pointer(n))) //nolint:gosec // see above
	return regErr(r)
}

func regErr(r uintptr) error {
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

var (
	hivePrivilegesOnce sync.Once
	errHivePrivileges  error
)

// enableHivePrivileges turns on SeBackupPrivilege and SeRestorePrivilege
// for the process, once.
func enableHivePrivileges() error {
	hivePrivilegesOnce.Do(func() {
		errHivePrivileges = enablePrivileges("SeBackupPrivilege", "SeRestorePrivilege")
	})
	return errHivePrivileges
}

func enablePrivileges(names ...string) error {
	var tok windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok)
	if err != nil {
		return err
	}
	defer func() { _ = tok.Close() }()
	for _, n := range names {
		var tp windows.Tokenprivileges
		tp.PrivilegeCount = 1
		tp.Privileges[0].Attributes = windows.SE_PRIVILEGE_ENABLED
		if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(n), &tp.Privileges[0].Luid); err != nil {
			return fmt.Errorf("look up %s: %w", n, err)
		}
		// It succeeds without granting a privilege the account lacks,
		// reporting that only as the last error.
		if err := procAdjustTokenPrivileges.Find(); err != nil {
			return err
		}
		r, _, lastErr := procAdjustTokenPrivileges.Call(uintptr(tok), 0,
			uintptr(unsafe.Pointer(&tp)), 0, 0, 0) //nolint:gosec // as for regLoadKey
		if r == 0 || errors.Is(lastErr, windows.ERROR_NOT_ALL_ASSIGNED) {
			return fmt.Errorf("enable %s: %w", n, lastErr)
		}
	}
	return nil
}
