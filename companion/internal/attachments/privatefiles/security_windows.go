//go:build windows

package privatefiles

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func PrepareDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("directory is unsafe")
	}
	if err := setCurrentUserOnly(path, true); err != nil {
		return err
	}
	return validateCurrentUserOnly(path)
}

func HardenFile(file *os.File) error {
	if file == nil {
		return errors.New("file is missing")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("file is unsafe")
	}
	if err := setCurrentUserOnly(file.Name(), false); err != nil {
		return err
	}
	return validateCurrentUserOnly(file.Name())
}

func ValidateFile(path string, info os.FileInfo) error {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("file is unsafe")
	}
	return validateCurrentUserOnly(path)
}

func SyncDirectory(string) error {
	// File.Sync and the write-through rename protect file contents on Windows.
	// Windows does not expose a portable directory fsync operation.
	return nil
}

func setCurrentUserOnly(path string, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	inheritance := uint32(windows.NO_INHERITANCE)
	if directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.ACCESS_MASK(windows.GENERIC_ALL),
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid),
		},
	}}, nil)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func validateCurrentUserOnly(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
	)
	if err != nil || descriptor == nil {
		return errors.New("security descriptor is unavailable")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("current user does not own path")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		return errors.New("path has unexpected access entries")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask == 0 {
		return errors.New("path access entry is unsafe")
	}
	entrySID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !entrySID.Equals(user.User.Sid) {
		return errors.New("path grants another principal access")
	}
	return nil
}
