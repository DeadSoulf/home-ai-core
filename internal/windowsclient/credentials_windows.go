//go:build windows

package windowsclient

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

type credentialW struct {
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

var (
	advapi32DLL   = windows.NewLazySystemDLL("advapi32.dll")
	procCredWrite = advapi32DLL.NewProc("CredWriteW")
	procCredRead  = advapi32DLL.NewProc("CredReadW")
	procCredDelete = advapi32DLL.NewProc("CredDeleteW")
	procCredFree  = advapi32DLL.NewProc("CredFree")
)

func SavePassword(server, username, password string) error {
	target, normalizedServer, normalizedUsername, err := credentialTarget(server, username)
	if err != nil {
		return err
	}
	if err := validateCredentialPassword(password); err != nil {
		return err
	}

	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	usernamePtr, err := windows.UTF16PtrFromString(normalizedUsername)
	if err != nil {
		return err
	}
	commentPtr, err := windows.UTF16PtrFromString("Home-AI password for " + normalizedUsername + " at " + normalizedServer)
	if err != nil {
		return err
	}

	blob := []byte(password)
	defer zeroBytes(blob)
	var blobPtr *byte
	if len(blob) > 0 {
		blobPtr = &blob[0]
	}
	credential := credentialW{
		Type:               credTypeGeneric,
		TargetName:         targetPtr,
		Comment:            commentPtr,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     blobPtr,
		Persist:            credPersistLocalMachine,
		UserName:           usernamePtr,
	}
	r1, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(targetPtr)
	runtime.KeepAlive(usernamePtr)
	runtime.KeepAlive(commentPtr)
	runtime.KeepAlive(blob)
	if r1 == 0 {
		return credentialCallError("CredWriteW", callErr)
	}
	return nil
}

func LoadPassword(server, username string) (string, error) {
	target, _, normalizedUsername, err := credentialTarget(server, username)
	if err != nil {
		return "", err
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return "", err
	}

	var credential *credentialW
	r1, _, callErr := procCredRead.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		credTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&credential)),
	)
	runtime.KeepAlive(targetPtr)
	if r1 == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrCredentialNotFound
		}
		return "", credentialCallError("CredReadW", callErr)
	}
	if credential == nil {
		return "", syscall.EINVAL
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(credential)))

	if credential.UserName == nil || windows.UTF16PtrToString(credential.UserName) != normalizedUsername {
		return "", errors.New("stored credential username does not match the requested Home-AI account")
	}
	if credential.CredentialBlobSize > 5*512 {
		return "", errors.New("stored credential blob exceeds the supported size")
	}
	if credential.CredentialBlobSize == 0 || credential.CredentialBlob == nil {
		return "", errors.New("stored credential password is empty")
	}

	blob := unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))
	passwordBytes := append([]byte(nil), blob...)
	zeroBytes(blob)
	defer zeroBytes(passwordBytes)
	password := string(passwordBytes)
	if err := validateCredentialPassword(password); err != nil {
		return "", err
	}
	return password, nil
}

func DeletePassword(server, username string) error {
	target, _, _, err := credentialTarget(server, username)
	if err != nil {
		return err
	}
	targetPtr, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	r1, _, callErr := procCredDelete.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		credTypeGeneric,
		0,
	)
	runtime.KeepAlive(targetPtr)
	if r1 == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return ErrCredentialNotFound
		}
		return credentialCallError("CredDeleteW", callErr)
	}
	return nil
}

func credentialCallError(operation string, err error) error {
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		err = syscall.EINVAL
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func zeroBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
