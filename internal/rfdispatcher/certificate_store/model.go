// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package certificate_store

import (
	"fmt"

	securestorage "stash.us.cray.com/HMS/hms-securestorage"
)

type CertificateStore struct {
	KeyPath       string
	SecureStorage securestorage.SecureStorage
}

func NewCertificateStore(keyPath string, ss securestorage.SecureStorage) *CertificateStore {
	return &CertificateStore{
		KeyPath:       keyPath,
		SecureStorage: ss,
	}
}

type CertificatePair struct {
	Certificate string
	PrivateKey  string
}

// Due to the sensitive nature of the data in Credential, make a custom String function to prevent certificates from being
// printed directly (accidentally) to output.
func (cred CertificatePair) String() string {
	return fmt.Sprint("PublicCertificate: <REDACTED>, TLS: <REDACTED>, X509: <REDACTED>")
}
