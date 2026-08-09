//go:build windows

package atomicfile

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// restrictFilePermissions replaces inherited access with a protected DACL for
// the current user, Local System, and local administrators. The temporary file
// remains open without delete sharing while its name is used to apply the ACL.
func restrictFilePermissions(file *os.File) error {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf(
		"D:P(A;;GA;;;%s)(A;;GA;;;SY)(A;;GA;;;BA)",
		user.User.Sid.String(),
	))
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		file.Name(),
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
}
