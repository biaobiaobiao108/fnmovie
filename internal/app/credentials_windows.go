//go:build windows

package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	credentialGeneric             = 1
	credentialPersistLocalMachine = 2
)

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	advapi32       = syscall.NewLazyDLL("Advapi32.dll")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

func credentialTarget(server string) string {
	return "FnMovie/" + strings.TrimRight(normalizeServerURL(server), "/")
}

func WriteCredential(server string, credential Credential) error {
	target, err := syscall.UTF16PtrFromString(credentialTarget(server))
	if err != nil {
		return err
	}
	username, err := syscall.UTF16PtrFromString(credential.Username)
	if err != nil {
		return err
	}
	blob, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	if len(blob) > 5120 {
		return fmt.Errorf("登录凭据长度超出 Windows 凭据管理器限制")
	}
	entry := windowsCredential{
		Type: credentialGeneric, TargetName: target,
		CredentialBlobSize: uint32(len(blob)), Persist: credentialPersistLocalMachine,
		UserName: username,
	}
	if len(blob) > 0 {
		entry.CredentialBlob = &blob[0]
	}
	ok, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&entry)), 0)
	if ok == 0 {
		return callErr
	}
	return nil
}

func ReadCredential(server string) (Credential, error) {
	target, err := syscall.UTF16PtrFromString(credentialTarget(server))
	if err != nil {
		return Credential{}, err
	}
	var entry *windowsCredential
	ok, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(target)), credentialGeneric, 0, uintptr(unsafe.Pointer(&entry)))
	if ok == 0 {
		return Credential{}, callErr
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(entry)))
	if entry == nil || entry.CredentialBlob == nil || entry.CredentialBlobSize == 0 {
		return Credential{}, fmt.Errorf("没有找到已保存的登录凭据")
	}
	blob := unsafe.Slice(entry.CredentialBlob, int(entry.CredentialBlobSize))
	var credential Credential
	if err := json.Unmarshal(blob, &credential); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func DeleteCredential(server string) error {
	target, err := syscall.UTF16PtrFromString(credentialTarget(server))
	if err != nil {
		return err
	}
	ok, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credentialGeneric, 0)
	if ok == 0 && callErr != syscall.Errno(1168) {
		return callErr
	}
	return nil
}
