// MIT License
//
// (C) Copyright 2022 Hewlett Packard Enterprise Development LP
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

// NOTE: This program is only intended for use with local testing, and not to be used
// in production.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/pdu_credential_store"
	securestorage "github.com/Cray-HPE/hms-securestorage"
)

type PDUs struct {
	Devices []pdu_credential_store.Device
}

func main() {
	// Open and parse the iPDUs.json file.
	jsonFile, err := os.Open("configs/iPDUs.json")
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	jsonBytes, _ := ioutil.ReadAll(jsonFile)
	var pdus PDUs
	err = json.Unmarshal(jsonBytes, &pdus)
	if err != nil {
		panic(err)
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
	pduCredentialStore := pdu_credential_store.NewPDUCredStore("", secureStorage)

	for _, pdu := range pdus.Devices {
		fmt.Println("Adding PDU:", pdu.Xname)
		err := pduCredentialStore.StorePDUCredentails(pdu)
		if err != nil {
			panic(err)
		}
	}
}
