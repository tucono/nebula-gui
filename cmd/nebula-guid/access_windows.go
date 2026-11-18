// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package main

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The local group calls and the session query aren't in x/sys/windows.
var (
	modnetapi32                 = windows.NewLazySystemDLL("netapi32.dll")
	procNetLocalGroupAdd        = modnetapi32.NewProc("NetLocalGroupAdd")
	procNetLocalGroupAddMembers = modnetapi32.NewProc("NetLocalGroupAddMembers")

	modwtsapi32                     = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInformationW = modwtsapi32.NewProc("WTSQuerySessionInformationW")
)

// Results of the local group calls that mean there's nothing to do.
const (
	errAliasExists   = windows.Errno(1379) // ERROR_ALIAS_EXISTS
	errMemberInAlias = windows.Errno(1378) // ERROR_MEMBER_IN_ALIAS
	nerrGroupExists  = windows.Errno(2223) // NERR_GroupExists
)

// WTS_INFO_CLASS values.
const (
	wtsUserName   = 5
	wtsDomainName = 7
)

// localGroupInfo1 is LOCALGROUP_INFO_1.
type localGroupInfo1 struct {
	name    *uint16
	comment *uint16
}

// localGroupMembersInfo3 is LOCALGROUP_MEMBERS_INFO_3.
type localGroupMembersInfo3 struct {
	domainAndName *uint16
}

// The calls below pass pointers to structs and strings the Windows API
// reads during the call, the documented use of unsafe.Pointer with
// syscall arguments.

// ensureGroup creates the local group if it doesn't exist yet.
func ensureGroup(name, comment string) error {
	info := localGroupInfo1{
		name:    windows.StringToUTF16Ptr(name),
		comment: windows.StringToUTF16Ptr(comment),
	}
	r, _, _ := procNetLocalGroupAdd.Call(0, 1, uintptr(unsafe.Pointer(&info)), 0) //nolint:gosec // see above
	switch err := windows.Errno(r); err {
	case 0, errAliasExists, nerrGroupExists:
		return nil
	default:
		return fmt.Errorf("create the %s group: %w", name, err)
	}
}

// addGroupMember adds a user (DOMAIN\name or name) to the local group. A
// user who's already a member is fine.
func addGroupMember(group, user string) error {
	m := localGroupMembersInfo3{domainAndName: windows.StringToUTF16Ptr(user)}
	g := windows.StringToUTF16Ptr(group)
	r, _, _ := procNetLocalGroupAddMembers.Call(0, uintptr(unsafe.Pointer(g)), 3, uintptr(unsafe.Pointer(&m)), 1) //nolint:gosec // see above
	switch err := windows.Errno(r); err {
	case 0, errMemberInAlias:
		return nil
	default:
		return fmt.Errorf("add %s to the %s group: %w", user, group, err)
	}
}

// errNoSessionUser means no one is signed in to this process's session,
// as for a process started by a service.
var errNoSessionUser = errors.New("no one is signed in to this session")

// sessionUser returns DOMAIN\name of the person signed in to the session
// this process runs in. An installer elevated with someone else's
// password (an administrator typing theirs) still runs in the session of
// the person at the screen, who is the one who'll use the app.
func sessionUser() (string, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return "", fmt.Errorf("find this session: %w", err)
	}
	user, err := sessionString(session, wtsUserName)
	if err != nil {
		return "", err
	}
	if user == "" {
		return "", errNoSessionUser
	}
	domain, err := sessionString(session, wtsDomainName)
	if err != nil {
		return "", err
	}
	if domain == "" {
		return user, nil
	}
	return domain + `\` + user, nil
}

func sessionString(session uint32, class uintptr) (string, error) {
	var buf *uint16
	var n uint32
	r, _, err := procWTSQuerySessionInformationW.Call(0, uintptr(session), class,
		uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n))) //nolint:gosec // see above
	if r == 0 {
		return "", fmt.Errorf("ask who is signed in: %w", err)
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buf))) //nolint:gosec // see above
	return windows.UTF16PtrToString(buf), nil
}
