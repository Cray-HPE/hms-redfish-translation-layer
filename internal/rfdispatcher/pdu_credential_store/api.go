// MIT License
//
// (C) Copyright [2019-2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

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
