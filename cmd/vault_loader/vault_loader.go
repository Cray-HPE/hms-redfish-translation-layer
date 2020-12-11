// Copyright 2019-2020 Hewlett Packard Enterprise Development LP

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
