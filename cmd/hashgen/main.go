package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/accountservice"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	password := flag.String("pass", "", "Password")

	flag.Parse()
	if *password == "" {
		flag.PrintDefaults()
		return
	}

	// GenerateFromPassword generates the salt and places it in the returned hash
	toHash := []byte(*password)
	hash, err := bcrypt.GenerateFromPassword(toHash, accountservice.PasswordHashCost)
	if err != nil {
		panic(err)
	}

	hashString := hex.EncodeToString(hash)

	fmt.Println("Password hash: " + hashString)
}
