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
	"fmt"

	securestorage "github.com/Cray-HPE/hms-securestorage"
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
