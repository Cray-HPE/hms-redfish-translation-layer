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
