// MIT License
//
// (C) Copyright [2018, 2021] Hewlett Packard Enterprise Development LP
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

/*
 * Account Service hooks
 * These hooks allows the Account Service to tie into the rfserver durning HTTP request handling.
 * Hooks are registered in the initAccountService function in rfserver.go
 *
 * When a HTTP request is made such as GET, PATCH, etc... the rfserver will:
 *  1. Execute any applicable before hooks on the request payload
 *  2. Perform the action of the request (such as update redis durning PATCH)
 *  3. Execute any applicable after hooks on the response payload
 *
 * The before and after hooks share a context map structure that allows the before hook to store data required
 * by the after hook (such as storing a password hash)
 */

package accountservice

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/hooks"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// Update is a hook that updates the in-memory copy of the account service
// This hook should be ran after PATCH or PUT requests on the AccountService
func (as *AccountService) Update(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	log.Debug("Updating account service")
	err := map2struct(response, as)
	log.Debug(as)
	return nil, 0, err
}

// AddAccountBefore is a hook, that does prep work for account creation
// This hook should be ran before POST requests on the Accounts collection
// TODO Make sure that RoleId is a valid value
func (mac *ManagerAccountCollection) AddAccountBefore(event hooks.Event, ctx, request map[string]interface{}) ([]byte, int, error) {
	log.Debug("Running Add account before")
	// When an account is created the Password field is required as defined in the schema.
	// Input validation is performed on the request for this required field, so if the password is missing something
	// is wrong
	passwordRaw, ok := request["Password"]
	if !ok {
		return nil, 0, errors.New("Password field not present in request")
	}
	password := passwordRaw.(string)
	passwordLen := int64(len(password))

	if passwordLen < mac.as.MinPasswordLength {
		// TODO: Custom HTTP messages like the once below should be moved into their own message registry
		msg := map[string]string{}
		msg["Message"] = "Supplied password is shorter then the minimum password length"
		msg["MessageId"] = "CustomHTTP.1.0.PasswordTooShort"
		msg["Resolution"] = "Supply a longer password"
		msg["Severity"] = "Critical"
		response, _ := json.Marshal(msg)
		return response, http.StatusBadRequest, errors.New("Supplied password is too short")
	} else if mac.as.MaxPasswordLength < passwordLen {
		msg := map[string]string{}
		msg["Message"] = "Supplied password is longer then the maximum password length"
		msg["MessageId"] = "CustomHTTP.1.0.PasswordTooLong"
		msg["Resolution"] = "Supply a shorter password"
		msg["Severity"] = "Critical"
		response, _ := json.Marshal(msg)
		return response, http.StatusBadRequest, errors.New("Supplied password is too long")
	}

	log.Debug("Generating password hash")
	// Note: The password hash contains both the hash and salt, and the bcrypt will generate both for us
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordHashCost)
	if err != nil {
		return nil, 0, err
	}

	// Store the hash in the context. AccountCollection.AddAccountAfter will pull the hash
	// from the context
	ctx["Password:hash"] = hash

	// Delete the password from the request, so that raw password is not stored
	delete(request, "Password")

	// Make sure that the Locked and Enabled properties are present
	if _, ok := request["Locked"]; !ok {
		// By default the account is not locked
		request["Locked"] = false
	}

	if _, ok := request["Enabled"]; !ok {
		// By default the account is enabled
		request["Enabled"] = true
	}

	log.Debug("Add account before done")
	return nil, 0, nil
}

// AddAccountAfter is a hook, that does finalizes account creation
// This hook should be ran after POST requests on the Accounts collection
func (mac *ManagerAccountCollection) AddAccountAfter(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	log.Debug("Running Add account after")
	passwordHashRaw, ok := ctx["Password:hash"]
	if !ok {
		return nil, 0, errors.New("Password hash missing from context")
	}
	passwordHash := passwordHashRaw.([]byte)

	account := NewManagerAccount(mac.as).initFromStruct(event.URI, response, passwordHash)
	_, present := mac.Members[account.UserName]
	if present {
		return nil, 0, errors.New("Account already exists in collection")
	}

	mac.Members[account.Id] = account

	// Password hashes are stored as hexadecimal strings in redis
	passwordHashString := hex.EncodeToString(passwordHash)
	passwordHashKey := event.URI + "/" + account.Id + "/Password:hash"
	log.Debug("Setting password hash for user: ", account.UserName)
	err := mac.as.rfd.Redis().Set(passwordHashKey, passwordHashString, 0).Err()
	if err != nil {
		log.Error("Error setting password hash key")
		return nil, 0, err
	}

	return nil, 0, nil
}

// UpdateAccountBefore is a hook that updates the account at this URI
// This hook should be ran before PATCH or PUT requests on the Accounts Collection
func (mac *ManagerAccountCollection) UpdateAccountBefore(event hooks.Event, ctx, request map[string]interface{}) ([]byte, int, error) {
	tokens := strings.Split(event.URI, "/")
	id := tokens[len(tokens)-1]

	log.Debug("Updating account with ID: ", id)

	account, ok := mac.Members[id]
	if !ok {
		return nil, 0, errors.New("Account does not exist with id: " + id)
	}

	// Store a reference to the account for UpdateAccountAfter
	ctx["Account"] = account

	// Perform Account's Update Before
	return account.UpdateBefore(event, ctx, request)
}

// UpdateAccountAfter is a hook that updates the copy of the account at this URI
// This hook should be ran after PATCH or PUT requests on the Accounts Collection
func (mac *ManagerAccountCollection) UpdateAccountAfter(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	accountRaw := ctx["Account"]
	account := accountRaw.(*ManagerAccount)

	return account.UpdateAfter(event, ctx, response)
}

// RemoveAccount will remove the account from the in-memory account collection. This will also clean up any special
// account service keys.
// This hook should be ran after DELETE requests on Accounts
func (mac *ManagerAccountCollection) RemoveAccount(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	// The response contains the old repsentation of the account being removed
	// Id is a required propertyand should be present in the response
	idRaw, ok := response["Id"]
	if !ok {
		return nil, 0, errors.New("Id is not present")
	}
	id := idRaw.(string)

	log.Debug("Removing account with Id: ", id)

	_, present := mac.Members[id]
	if !present {
		return nil, 0, errors.New("Account does not exist is collection")
	}

	delete(mac.Members, id)

	passwordHashKey := event.URI + "/Password:hash"
	log.Debug("Deleting password hash for ", id)
	err := mac.as.rfd.Redis().Del(passwordHashKey).Err()
	if err != nil {
		log.Error("Error deleting password hash key")
		return nil, 0, err
	}

	return nil, 0, nil
}

// UpdateBefore will do prep work for updating this account. Such as generating the password hash from the request and
// storing it in the hook context. Also it will remove the Password field from the request
// This hook should be ran before PATCH or PUT requests on Accounts
func (ma *ManagerAccount) UpdateBefore(event hooks.Event, ctx, request map[string]interface{}) ([]byte, int, error) {
	log.Debug("Running Account Update before")
	// When an account is being updated a new password could be supplied

	log.Debug("Request: ", request)

	// TODO This is not working
	passwordRaw, ok := request["Password"]
	log.Debug("Password present in request: ", ok)
	if ok {
		log.Debug("Password present in request")
		password := passwordRaw.(string)
		passwordLen := int64(len(password))
		if passwordLen < ma.as.MinPasswordLength {
			// TODO: Custom HTTP messages like the once below should be moved into their own message registry
			msg := map[string]string{}
			msg["Message"] = "Supplied password is shorter then the minimum password length"
			msg["MessageId"] = "CustomHTTP.1.0.PasswordTooShort"
			msg["Resolution"] = "Supply a longer password"
			msg["Severity"] = "Critical"
			response, _ := json.Marshal(msg)
			return response, http.StatusBadRequest, errors.New("Supplied password is too short")
		} else if ma.as.MaxPasswordLength < passwordLen {
			msg := map[string]string{}
			msg["Message"] = "Supplied password is longer then the maximum password length"
			msg["MessageId"] = "CustomHTTP.1.0.PasswordTooLong"
			msg["Resolution"] = "Supply a shorter password"
			msg["Severity"] = "Critical"
			response, _ := json.Marshal(msg)
			return response, http.StatusBadRequest, errors.New("Supplied password is too long")
		}

		log.Debug("Generating password hash")
		hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordHashCost)
		if err != nil {
			return nil, 0, err
		}

		// Store the hash in the context. ManagerAccount.UpdateAfter will pull the hash
		// from the context
		ctx["Password:hash"] = hash

		// Delete the password from the request, so that raw password is not stored
		delete(request, "Password")
	}

	// Check to see if username is being set if so, check that it is unique
	if usernameRaw, ok := request["UserName"]; ok {
		username := usernameRaw.(string)
		if _, present := ma.as.Accounts.Members[username]; present {
			// TODO: There should be a better way to send errors like this back to the user
			// currently it will only output to the console, will not get an message annotation
			// Could place a invalid username key into the context map and use UpdateAfter to add the
			// annotation
			log.Warn("Duplicate username, removing username from request")
			ctx["DuplicateUserName"] = true

			// Remove this field from the request
			delete(request, "UserName")
		}
	}

	log.Debug("Account Update before done")
	return nil, 0, nil
}

// UpdateAfter will update the in-memory account. This will also set special keys such as the password hash
// This hook should be ran after PATCH or PUT requests on Accounts
func (ma *ManagerAccount) UpdateAfter(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	log.Debug("Running Account Update after")

	err := map2struct(response, ma)
	if err != nil {
		return nil, 0, err
	}

	if passwordHashRaw, ok := ctx["Password:hash"]; ok {
		passwordHash := passwordHashRaw.([]byte)
		log.Debug("Updating Password hash for user: ", ma.UserName)

		// Update the users password
		ma.passwordHash = passwordHash

		// Set password hash in redis
		passwordHashString := hex.EncodeToString(passwordHash)
		passwordHashKey := event.URI + "/" + ma.Id + "/Password:hash"
		log.Debug("Setting password hash for user: ", ma.UserName)
		err := ma.as.rfd.Redis().Set(passwordHashKey, passwordHashString, 0).Err()
		if err != nil {
			log.Error("Error setting password hash key")
			return nil, 0, err
		}

	}

	if _, ok := ctx["DuplicateUserName"]; ok {
		// TODO: Add a Message annotation on the UserName field to explain that the username already exists
		// This could be done by returning a response body with a Message annotation added on
	}

	log.Debug("Account Update after done")

	return nil, 0, nil
}
