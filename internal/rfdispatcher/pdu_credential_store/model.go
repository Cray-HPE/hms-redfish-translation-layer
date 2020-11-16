// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package pdu_credential_store

import (
	"fmt"

	securestorage "stash.us.cray.com/HMS/hms-securestorage"
)

type PDUCredentialStore struct {
	KeyPath       string
	SecureStorage securestorage.SecureStorage
}

func NewPDUCredStore(keyPath string, ss securestorage.SecureStorage) *PDUCredentialStore {
	return &PDUCredentialStore{
		KeyPath:       keyPath,
		SecureStorage: ss,
	}
}

type Device struct {
	Xname    string `json:"xname"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Due to the sensitive nature of the data in Device, make a custom String function to prevent passwords from being
// printed directly (accidentally) to output.
func (device Device) String() string {
	return fmt.Sprintf("URL: %s, Username: %s, Password: <REDACTED>", device.URL, device.Username)
}

type DefaultCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Due to the sensitive nature of the data in Device, make a custom String function to prevent passwords from being
// printed directly (accidentally) to output.
func (cred DefaultCredential) String() string {
	return fmt.Sprintf("Username: %s, Password: <REDACTED>", cred.Username)
}
