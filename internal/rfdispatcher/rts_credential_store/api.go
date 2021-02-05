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
