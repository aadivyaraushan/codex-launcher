//go:build windows

package configsecurity

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func prepareRoot(root string) error {
	if err := os.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return ErrUnsafe
	}
	if err := applyOwnerOnly(root); err != nil {
		return ErrUnsafe
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafe
	}
	return validateOwnerOnly(root)
}

func publishNew(temporaryPath, path string) error {
	if err := applyOwnerOnly(temporaryPath); err != nil {
		return ErrUnsafe
	}
	if err := os.Link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrExists
		}
		return ErrUnsafe
	}
	return nil
}

func publishReplace(temporaryPath, path string) error {
	if err := applyOwnerOnly(temporaryPath); err != nil {
		return ErrUnsafe
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return ErrUnsafe
	}
	return nil
}

func syncRoot(string) error {
	return nil
}

func validateRoot(path string, _ os.FileInfo) error {
	return validateOwnerOnly(path)
}

func validateFile(path string, _ os.FileInfo) error {
	return validateOwnerOnly(path)
}

func applyOwnerOnly(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return ErrUnsafe
	}
	descriptor, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "D:P(A;;FA;;;SY)(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return ErrUnsafe
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return ErrUnsafe
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, user.User.Sid, nil, dacl, nil); err != nil {
		return ErrUnsafe
	}
	return nil
}

func validateOwnerOnly(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return ErrUnsafe
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		return ErrUnsafe
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return ErrUnsafe
	}
	actualDACL := daclSection(descriptor.String())
	wantUser := "(A;;FA;;;" + user.User.Sid.String() + ")"
	if actualDACL != "D:P(A;;FA;;;SY)"+wantUser && actualDACL != "D:P"+wantUser+"(A;;FA;;;SY)" {
		return ErrUnsafe
	}
	return nil
}

func daclSection(sddl string) string {
	start := strings.Index(sddl, "D:")
	if start < 0 {
		return ""
	}
	section := sddl[start:]
	if end := strings.Index(section[2:], "S:"); end >= 0 {
		section = section[:end+2]
	}
	return section
}
