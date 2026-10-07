//go:build !windows

package app

import "errors"

type Credential struct {
	Username string
	Password string
	Token    string
}

func WriteCredential(string, Credential) error {
	return errors.New("Windows credential storage is only available on Windows")
}
func ReadCredential(string) (Credential, error) {
	return Credential{}, errors.New("Windows credential storage is only available on Windows")
}
func DeleteCredential(string) error { return nil }
