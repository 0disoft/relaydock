//go:build windows

package storage_test

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func assertPrivateStateFile(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("state file inherits access instead of using a protected DACL")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sddl := descriptor.String()
	if !strings.Contains(sddl, user.User.Sid.String()) {
		t.Fatalf("state file DACL does not grant the current user access: %s", sddl)
	}
	for _, broadTrustee := range []string{";;;WD)", ";;;AU)", ";;;BU)"} {
		if strings.Contains(sddl, broadTrustee) {
			t.Fatalf("state file DACL grants broad access through %s: %s", broadTrustee, sddl)
		}
	}
}
