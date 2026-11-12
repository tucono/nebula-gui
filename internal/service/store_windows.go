// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package service

import "golang.org/x/sys/windows"

// stateDirSDDL is a protected DACL giving full control to SYSTEM and
// Administrators only, inherited by files and subdirectories.
const stateDirSDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// restrictDir replaces dir's DACL so other users can't read the config.
// Files created in it inherit the DACL.
func restrictDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(stateDirSDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
}
