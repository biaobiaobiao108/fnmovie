package app

import (
	"encoding/json"
	"errors"

	"github.com/egoist/mygo"
)

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

func WriteCredential(server string, credential Credential) error {
	data, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	return mygo.Secrets.Set(normalizeServerURL(server), data)
}

func ReadCredential(server string) (Credential, error) {
	data, err := mygo.Secrets.Get(normalizeServerURL(server))
	if err != nil {
		return Credential{}, err
	}
	var credential Credential
	err = json.Unmarshal(data, &credential)
	return credential, err
}

func DeleteCredential(server string) error {
	err := mygo.Secrets.Delete(normalizeServerURL(server))
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return nil
	}
	return err
}
