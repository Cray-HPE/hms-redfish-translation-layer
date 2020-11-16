// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package rts_credential_store

import (
	"fmt"

	securestorage "stash.us.cray.com/HMS/hms-securestorage"
)

type RTSCredentialStore struct {
	KeyPath       string
	SecureStorage securestorage.SecureStorage
}

func NewRTSCredStore(keyPath string, ss securestorage.SecureStorage) *RTSCredentialStore {
	return &RTSCredentialStore{
		KeyPath:       keyPath,
		SecureStorage: ss,
	}
}

type Credential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Due to the sensitive nature of the data in Credential, make a custom String function to prevent passwords from being
// printed directly (accidentally) to output.
func (cred Credential) String() string {
	return fmt.Sprintf("Username: %s, Password: <REDACTED>", cred.Username)
}
