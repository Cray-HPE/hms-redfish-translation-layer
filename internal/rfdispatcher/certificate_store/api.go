// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package certificate_store

import (
	"errors"
	"path"
)

var ErrNotFound = errors.New("certificate pair not found")

func (credStore *CertificateStore) GetCertificatePair(xname string) (pair CertificatePair, err error) {
	err = credStore.SecureStorage.Lookup(path.Join(credStore.KeyPath, xname), &pair)

	if pair == (CertificatePair{}) {
		err = ErrNotFound
		return
	}

	if pair.Certificate == "" || pair.PrivateKey == "" {
		err = errors.New("empty certificate or private key")
	}
	return
}

func (credStore *CertificateStore) StoreCertificatePair(xname string, pair CertificatePair) error {
	if pair.Certificate == "" || pair.PrivateKey == "" {
		return errors.New("empty certificate or private key")
	}

	return credStore.SecureStorage.Store(path.Join(credStore.KeyPath, xname), pair)
}
