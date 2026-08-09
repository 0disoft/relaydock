//go:build windows

package credentials

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/0disoft/relaydock/internal/core"
	"golang.org/x/sys/windows"
)

const (
	windowsCredentialTypeGeneric         = 1
	windowsCredentialPersistLocalMachine = 2
)

var (
	advapi32DLL     = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32DLL.NewProc("CredWriteW")
	procCredReadW   = advapi32DLL.NewProc("CredReadW")
	procCredDeleteW = advapi32DLL.NewProc("CredDeleteW")
	procCredFree    = advapi32DLL.NewProc("CredFree")
)

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsCredentialBackend struct{}

func newPlatformCredentialBackend() (systemCredentialBackend, error) {
	expectedCredentialSize := uintptr(48)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		expectedCredentialSize = 80
	}
	if actual := unsafe.Sizeof(windowsCredential{}); actual != expectedCredentialSize {
		return nil, fmt.Errorf("%w: unexpected Windows CREDENTIALW layout size %d", ErrSystemStoreUnavailable, actual)
	}
	if err := advapi32DLL.Load(); err != nil {
		return nil, fmt.Errorf("%w: load Windows Credential Manager: %v", ErrSystemStoreUnavailable, err)
	}
	return windowsCredentialBackend{}, nil
}

func (windowsCredentialBackend) Write(target string, value []byte) error {
	targetName, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("encode credential target: %w", err)
	}
	username, err := windows.UTF16PtrFromString("RelayDock")
	if err != nil {
		return fmt.Errorf("encode credential username: %w", err)
	}
	if len(value) == 0 {
		return fmt.Errorf("%w: empty credential", core.ErrInvalidArgument)
	}
	credential := windowsCredential{
		Type: windowsCredentialTypeGeneric, TargetName: targetName,
		CredentialBlobSize: uint32(len(value)), CredentialBlob: &value[0],
		Persist: windowsCredentialPersistLocalMachine, UserName: username,
	}
	result, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(targetName)
	runtime.KeepAlive(username)
	runtime.KeepAlive(value)
	if result == 0 {
		return windowsCredentialError(callErr)
	}
	return nil
}

func (windowsCredentialBackend) Read(target string) ([]byte, error) {
	targetName, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return nil, fmt.Errorf("encode credential target: %w", err)
	}
	var credential *windowsCredential
	result, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetName)), windowsCredentialTypeGeneric, 0,
		uintptr(unsafe.Pointer(&credential)),
	)
	runtime.KeepAlive(targetName)
	if result == 0 {
		return nil, windowsCredentialError(callErr)
	}
	if credential == nil {
		return nil, fmt.Errorf("%w: Windows returned an empty credential", core.ErrInvalidConfiguration)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.CredentialBlobSize == 0 || credential.CredentialBlob == nil || credential.CredentialBlobSize > maximumSystemCredentialBytes {
		return nil, fmt.Errorf("%w: Windows returned an empty credential value", core.ErrInvalidConfiguration)
	}
	value := unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))
	return append([]byte(nil), value...), nil
}

func (windowsCredentialBackend) Delete(target string) error {
	targetName, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("encode credential target: %w", err)
	}
	result, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetName)), windowsCredentialTypeGeneric, 0,
	)
	runtime.KeepAlive(targetName)
	if result == 0 {
		return windowsCredentialError(callErr)
	}
	return nil
}

func windowsCredentialError(err error) error {
	if errors.Is(err, windows.ERROR_NOT_FOUND) {
		return core.ErrNotFound
	}
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return fmt.Errorf("Windows Credential Manager operation failed")
	}
	return err
}
