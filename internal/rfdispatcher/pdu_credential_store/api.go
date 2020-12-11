// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

package pdu_credential_store

import (
	"errors"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
)

// CredentialsGlobalKey is the Vault key used to access RTS global credentials
const CredentialsGlobalKey = "global/pdu"

func (credStore *PDUCredentialStore) SetKeypathValue(data map[string]interface{}) (err error) {
	err = credStore.SecureStorage.Store(credStore.KeyPath, data)

	return
}

func (credStore *PDUCredentialStore) GetDevices() (devices map[string]Device, err error) {
	baseKey := credStore.KeyPath

	log.WithField("baseKey", baseKey).Debug("Looking up ALL PDU Device keys")
	keys, err := credStore.SecureStorage.LookupKeys(baseKey)
	if err != nil {
		log.WithFields(log.Fields{
			"baseKey": baseKey,
			"err":     err,
		}).Error("Unable to list PDU keys")
		return nil, err
	}

	devices = make(map[string]Device)
	for _, key := range keys {
		if strings.Contains(key, "global") || strings.Contains(key, "certificates") {
			// Ignore the global or certificate keys, as that is not a PDU
			continue
		}

		pduKey := path.Join(baseKey, key)
		log.WithField("pduKey", pduKey).Debug("Looking up PDU Device keys")

		var device Device
		err = credStore.SecureStorage.Lookup(pduKey, &device)
		if err != nil {
			log.WithFields(log.Fields{
				"pduKey": pduKey,
				"err":    err,
			}).Error("Unable to lookup PDU Device key")
			// Not sure if this is the best course of action, but for now we'll just take what we can get.
			continue
		}
		devices[device.Xname] = device
	}

	return devices, nil
}

func (credStore *PDUCredentialStore) GetDefaultPDUCredentails() (cred DefaultCredential, err error) {
	key := path.Join(credStore.KeyPath, CredentialsGlobalKey)
	err = credStore.SecureStorage.Lookup(key, &cred)
	if err != nil {
		return
	}

	if cred.Password == "" || cred.Username == "" {
		err = errors.New("empty username or password")
	}
	return
}

func (credStore *PDUCredentialStore) StoreDefaultPDUCredentails(cred DefaultCredential) error {
	if cred.Password == "" || cred.Username == "" {
		return errors.New("empty username or password")
	}

	key := path.Join(credStore.KeyPath, CredentialsGlobalKey)
	return credStore.SecureStorage.Store(key, cred)
}

func (credStore *PDUCredentialStore) StorePDUCredentails(cred Device) error {
	if cred.Xname == "" {
		return errors.New("empty xname")
	}

	if cred.URL == "" {
		return errors.New("empty device url")
	}

	if cred.Password == "" || cred.Username == "" {
		return errors.New("empty username or password")
	}

	key := path.Join(credStore.KeyPath, cred.Xname)
	return credStore.SecureStorage.Store(key, cred)
}
