// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package rts_credential_store

import (
	"errors"
	"path"
)

// CredentialsGlobalKey is the Vault key used to access RTS global credentials
const CredentialsGlobalKey = "global/rts"

func (credStore *RTSCredentialStore) GetGlobalCredentials() (cred Credential, err error) {
	err = credStore.SecureStorage.Lookup(path.Join(credStore.KeyPath, CredentialsGlobalKey), &cred)

	if cred.Password == "" || cred.Username == "" {
		err = errors.New("empty username or password")
	}
	return
}

func (credStore *RTSCredentialStore) StoreGlobalCredentials(cred Credential) error {
	if cred.Password == "" || cred.Username == "" {
		return errors.New("empty username or password")
	}

	return credStore.SecureStorage.Store(path.Join(credStore.KeyPath, CredentialsGlobalKey), cred)
}
