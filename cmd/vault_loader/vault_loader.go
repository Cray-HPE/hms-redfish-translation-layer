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

package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/pdu_credential_store"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/rts_credential_store"
	securestorage "stash.us.cray.com/HMS/hms-securestorage"
)

func main() {
	defaultRTSCredentails, ok := os.LookupEnv("VAULT_RTS_DEFAULT_CREDENTIALS")
	if !ok {
		log.Fatal("Value not set for VAULT_RTS_DEFAULT_CREDENTIALS")
	}

	defaultPDUCredentails, ok := os.LookupEnv("VAULT_PDU_DEFAULT_CREDENTIALS")
	if !ok {
		log.Fatal("Value not set for VAULT_PDU_DEFAULT_CREDENTIALS")
	}

	// Setup Vault. It's kind of a big deal, so we'll wait forever for this to work.
	var secureStorage securestorage.SecureStorage
	vaultKeypath, ok := os.LookupEnv("JAWS_VAULT_KEYPATH")
	if !ok {
		log.Fatal("Value not set for JAWS_VAULT_KEYPATH")
	}

	for {
		var err error
		// Start a connection to Vault
		if secureStorage, err = securestorage.NewVaultAdapter(vaultKeypath); err == nil {
			log.Println("Connected to Vault. vaultKeypath:", vaultKeypath)
			break
		}

		log.Println("Unable to connect to Vault ...trying again in 5 seconds. Err:", err)
		time.Sleep(5 * time.Second)
	}

	// Setup Credential stores
	rtsCredentialStore := rts_credential_store.NewRTSCredStore("", secureStorage)
	pduCredentialStore := pdu_credential_store.NewPDUCredStore("", secureStorage)

	// RTS Defaults
	log.Println("Storing RTS default credentails")
	rtsCredentails := rts_credential_store.Credential{}
	err := json.Unmarshal([]byte(defaultRTSCredentails), &rtsCredentails)
	if err != nil {
		log.Fatalln("Unable to unmarshal defaults for RTS. Err:", err)
	}

	err = rtsCredentialStore.StoreGlobalCredentials(rtsCredentails)
	if err != nil {
		log.Fatalln("Unable to store defaults for RTS. Err:", err)
	}

	// PDU Defaults
	log.Println("Storing iPDU default credentails")
	pduCredentials := pdu_credential_store.DefaultCredential{}
	err = json.Unmarshal([]byte(defaultPDUCredentails), &pduCredentials)
	if err != nil {
		log.Fatalln("Unable to unmarshal defaults for iPDUs. Err:", err)
	}

	err = pduCredentialStore.StoreDefaultPDUCredentails(pduCredentials)
	if err != nil {
		log.Fatalln("Unable to store defaults for iPDUs. Err:", err)
	}

	log.Println("Done.")
}
