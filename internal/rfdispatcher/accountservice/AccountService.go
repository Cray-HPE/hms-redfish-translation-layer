// MIT License
//
// (C) Copyright [2018-2023,2025] Hewlett Packard Enterprise Development LP
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
 * Account Service
 * This file implements the redfish account service which handles account authentication and premissions
 * See the file: AccountServiceHooks.go for callback functions executed by the rfserver.
 */

package accountservice

import (
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/dispatcher"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfschema"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// PasswordHashCost is the cost used by bcrypt when generating new password hashes
const PasswordHashCost = bcrypt.DefaultCost

// AccountService is the entry point for account management. This a partial representation of the redfish
// account service as only the required fields are presented here.
type AccountService struct {
	AccountLockoutCounterResetAfter int64
	AccountLockoutDuration          int64
	AccountLockoutThreshold         int64

	MinPasswordLength int64
	MaxPasswordLength int64
	ServiceEnabled    bool // TODO Respect

	Accounts *ManagerAccountCollection
	Roles    *RoleCollection

	rfd    *dispatcher.RedfishDispatcher
	schema *rfschema.Resource
}

func NewAccountService(rfd *dispatcher.RedfishDispatcher) *AccountService {
	as := &AccountService{}
	as.rfd = rfd
	return as
}

// InitFromRedis will initialize this account server instance with data in redis
func (as *AccountService) InitFromRedis() {
	log.Trace("Initializing Account Service")

	uri := "/redfish/v1/AccountService"
	resource, property := as.rfd.GetResource(uri, "")
	if resource == nil {
		panic("Account Service resource is nil")
	}
	if property != nil {
		panic("Account Service URI returned a property")
	}

	as.schema = resource

	r2s := dispatcher.Redis2Interface{
		RFD:      as.rfd,
		Resource: resource,
		URI:      uri,
	}
	raw, err := r2s.BuildStruct(resource.Properties, uri)
	if err != nil {
		panic(err)
	}

	// TODO: Replace with map2struct
	as.AccountLockoutDuration = raw["AccountLockoutDuration"].(int64)
	as.AccountLockoutCounterResetAfter = raw["AccountLockoutCounterResetAfter"].(int64)
	as.AccountLockoutThreshold = raw["AccountLockoutThreshold"].(int64)
	as.MinPasswordLength = raw["MinPasswordLength"].(int64)
	as.MaxPasswordLength = raw["MaxPasswordLength"].(int64)
	as.ServiceEnabled = raw["ServiceEnabled"].(bool)

	as.Accounts = NewManagerAccountCollection(as).initFromRedis()
	as.Roles = NewRoleCollection(as).initFromRedis()

	log.Trace("Account Service initialization complete")
}

// AuthenticateAccount will attempt to authenticate the given account username and password.
// If successful a reference to Account will be returned, otherwise an error explaing what went wrong will be
// returned.
func (as *AccountService) AuthenticateAccount(username, password string) (*ManagerAccount, error) {
	// Does the user exist?
	account, ok := as.Accounts.get(username)
	if !ok {
		return nil, errors.New("Account with provided user name does not exist")
	}

	if err := account.Authenticate(password); err != nil {
		// Failed authentication
		return nil, err
	}

	// The user is authenticated
	log.WithField("username", username).Debug("User authenticated")
	return account, nil
}

func (as *AccountService) isAccountLockoutEnabled() bool {
	return as.AccountLockoutThreshold != 0 && as.AccountLockoutDuration != 0
}

// RunPeriodic will execute the AccountService's updatePeriodic routine every second
// and updateAccountsPeriodic routine every minute.
// Please note this should be ran as a go routine, as this function will never return
func (as *AccountService) RunPeriodic() {
	log.Info("Starting account service periodic task")

	// Manually run the first updates so we can immediately handle requests
	as.updatePeriodic()
	as.updateAccountsPeriodic()

	ticker := time.NewTicker(1 * time.Second)
	accountTicker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			as.updatePeriodic()
		case <-accountTicker.C:
			as.updateAccountsPeriodic()
		}
	}
}

// updatePeriodic will the run periodic updates required by the account service.
// Such as updating the state of locked accounts.
func (as *AccountService) updatePeriodic() {
	log.Trace("Account Service updatePeriodic running")

	as.Accounts.updateMux.Lock()
	defer as.Accounts.updateMux.Unlock()
	//log.Trace("Update periodic running")

	// Account lockout check
	for _, account := range as.Accounts.Members {
		if err := account.updatePeriodic(); err != nil {
			log.Error(err)
		}
	}

}

// updateAccountsPeriodic will the run periodic updates to pick up account changes in redis.
func (as *AccountService) updateAccountsPeriodic() {
	log.Trace("Account Service updateAccountsPeriodic running")

	// Check for changes to the accounts collection
	as.Accounts.initFromRedis()

}

// ManagerAccountCollection manages the creation, update, and removal of ManagerAccounts
type ManagerAccountCollection struct {
	Members map[string]*ManagerAccount

	as *AccountService
	updateMux sync.Mutex
}

func NewManagerAccountCollection(as *AccountService) *ManagerAccountCollection {
	mac := &ManagerAccountCollection{}
	mac.Members = map[string]*ManagerAccount{}
	mac.as = as
	return mac
}

// initFromRedis will initialize this collection (and its member accounts) from data in redis
func (mac *ManagerAccountCollection) initFromRedis() *ManagerAccountCollection {
	log.Trace("Initializing Manager Account Collection")

	mac.updateMux.Lock()
	defer mac.updateMux.Unlock()
	uri := "/redfish/v1/AccountService/Accounts"
	resource, property := mac.as.rfd.GetResource(uri, "")
	if resource == nil {
		panic("Manager Account Collection resource is nil")
	}
	if property != nil {
		panic("Manager Account Collection URI returned a property")
	}

	r2s := dispatcher.Redis2Interface{
		RFD:      mac.as.rfd,
		Resource: resource,
		URI:      uri,
	}
	raw, err := r2s.BuildStruct(resource.Properties, uri)
	if err != nil {
		panic(err)
	}

	if raw["Members"] == nil {
		// TODO CASMHMS-5510: How AccountService works in RTS will need to change in the future.
		// For now if there are no account defined in REDIS, then we should fail.
		log.Fatal("Account Collection is empty. Did the backend helpers not create an account? ")
	}

	members := raw["Members"].([]interface{})

	for _, memberRaw := range members {
		member := memberRaw.(map[string]interface{})
		accountURI := member["@odata.id"].(string)
		newAccount := NewManagerAccount(mac.as).initFromRedis(accountURI)

		account, ok := mac.Members[newAccount.UserName]
		if !ok {
			// Add it to the collection
			mac.Members[newAccount.UserName] = newAccount
		} else {
			// Only update the password from redis
			account.passwordHash = newAccount.passwordHash
		}
	}
	log.Debug("Manager Account Collection initialization complete")
	// Return itself
	return mac
}

// get will return the account with the provided username
func (mac *ManagerAccountCollection) get(username string) (*ManagerAccount, bool) {
	mac.updateMux.Lock()
	defer mac.updateMux.Unlock()
	// TODO: Add a username2account lookup table for something if we expect there to a lot of accounts and
	// account lookups
	for _, account := range mac.Members {
		if account.UserName == username {
			return account, true
		}
	}

	return nil, false
}

// ManagerAccount is a partial representation of the redfish ManagerAccount schema.
// This structure also contains infomation needed for account lock outs
type ManagerAccount struct {
	Id       string
	UserName string
	RoleId   string
	Locked   bool
	Enabled  bool

	passwordHash []byte

	failedLoginMux      sync.Mutex // Used control access to an account when updating lock out information
	failedLoginAttempts int64
	lastFailedLogin     time.Time
	lockOutStart        time.Time

	as *AccountService
}

func NewManagerAccount(as *AccountService) *ManagerAccount {
	ma := &ManagerAccount{}
	ma.as = as
	return ma
}

// initFromRedis will initialize this account from data in redis
func (ma *ManagerAccount) initFromRedis(uri string) *ManagerAccount {
	log.WithFields(log.Fields{
		"uri": uri,
	}).Trace("Initializing Manager Account")

	resource, property := ma.as.rfd.GetResource(uri, "")
	if resource == nil {
		log.Fatal("Manager Account resource is nil")
	}
	if property != nil {
		log.Fatal("Manager Account URI returned a property")
	}

	r2s := dispatcher.Redis2Interface{
		RFD:      ma.as.rfd,
		Resource: resource,
		URI:      uri,
	}
	raw, err := r2s.BuildStruct(resource.Properties, uri)
	if err != nil {
		panic(err)
	}

	// Need to query redis for the password hash
	passwordHashKey := uri + "/Password:hash"
	passwordHashRaw, err := ma.as.rfd.Redis().Get(passwordHashKey).Result()
	if err != nil {
		log.Fatal("Unable to retrieve password hash")
	}
	// The Password hash in redis is stored as a hexadecimal string
	passwordHash, err := hex.DecodeString(passwordHashRaw)
	if err != nil {
		panic(err)
	}

	ma.initFromStruct(uri, raw, passwordHash)

	log.WithFields(log.Fields{
		"account": ma.UserName,
		"uri":     uri,
	}).Debug("Manager Account loaded")
	// Return itself
	return ma
}

// initFromStruct will initialize this account from data contained in a map
func (ma *ManagerAccount) initFromStruct(uri string, raw map[string]interface{}, passwordHash []byte) *ManagerAccount {
	// TODO: Use map2struct
	ma.Id = raw["Id"].(string)
	ma.UserName = raw["UserName"].(string)
	ma.RoleId = raw["RoleId"].(string)
	ma.Locked = raw["Locked"].(bool)
	ma.Enabled = raw["Enabled"].(bool)
	ma.passwordHash = passwordHash

	return ma
}

// Authenticate will compare the provided password. If the password is correct nil is returned. Otherwise an error
// if the account can not be authenticated
// TODO: Regenerate password hash if the bycrpt difficulty changes upon a successful login
// TODO: Explore the idea of reseting the failed login counter upon a users successful login
func (ma *ManagerAccount) Authenticate(password string) error {
	if ma.Locked {
		log.WithField("username", ma.UserName).Error("Attempted login by locked account")
		return errors.New("Login by locked account")
	}

	// Note: The password hash contains both the hash and salt, and the bcrypt will extract it for us
	err := bcrypt.CompareHashAndPassword(ma.passwordHash, []byte(password))
	if err != nil {
		log.WithField("username", ma.UserName).Error("Invalid password provided for user")

		err := ma.failedLogin()
		if err != nil {
			log.Error(err)
			return err
		}

		return errors.New("Invalid password")
	}

	return nil
}

// failedLogin will record the failed long if enabled by the account service. If required the accounts lockout sate
// will updated.
func (ma *ManagerAccount) failedLogin() error {
	if !ma.as.isAccountLockoutEnabled() {
		// Account lockouts are currently disabled
		log.WithField("username", ma.UserName).Debug(
			"Not recording failed login due to lockouts being disabled")
		return nil
	}

	// A accounts failed lockout state can only be altered by a single go routine at any given time
	ma.failedLoginMux.Lock()
	defer ma.failedLoginMux.Unlock()

	// Account lockout update
	ma.failedLoginAttempts++
	ma.lastFailedLogin = time.Now()
	log.WithFields(log.Fields{
		"username":            ma.UserName,
		"failedLoginAttempts": ma.failedLoginAttempts,
	}).Debug("Account failed login attempts")
	if ma.as.AccountLockoutThreshold < ma.failedLoginAttempts {
		log.WithField("username", ma.UserName).Warning("Account is now locked")
		// Update account
		ma.lockOutStart = time.Now()
		if err := ma.setLockOut(true); err != nil {
			log.Error(err)
			return err
		}
	}

	return nil
}

// setLockOut will set the account locked out value.
// Note: This is the preferred way to set whether an account is locked out, as this will also set the key in redis
func (ma *ManagerAccount) setLockOut(value bool) error {
	ma.Locked = value
	// TODO: Can this key be created dynamically some how?
	accountKey := "/redfish/v1/AccountService/Accounts/" + ma.Id
	update := map[string]interface{}{
		"Locked": strconv.FormatBool(ma.Locked),
	}

	return ma.as.rfd.RedisPatch(update, accountKey)
}

// updatePeriodic will update the account's current lockout state.
func (ma *ManagerAccount) updatePeriodic() error {
	log.WithFields(log.Fields{
		"ID":                  ma.Id,
		"UserName":            ma.UserName,
		"RoleId":              ma.RoleId,
		"Locked":              ma.Locked,
		"Enabled":             ma.Enabled,
		"failedLoginAttempts": ma.failedLoginAttempts,
		"lastFailedLogin":     ma.lastFailedLogin,
		"lockOutStart":        ma.lockOutStart,
	}).Trace("Manager Account updatePeriodic running")

	ma.failedLoginMux.Lock()
	defer ma.failedLoginMux.Unlock()

	if ma.failedLoginAttempts != 0 {
		// If AccountLockoutCounterReset has elasped reset counter
		counterResetDuration := time.Duration(ma.as.AccountLockoutCounterResetAfter) * time.Second
		timeSinceLastFailedLogin := time.Since(ma.lastFailedLogin)

		log.Debug("Counter Reset Duration: ", counterResetDuration)
		log.Debug("Time since last failed login: ", timeSinceLastFailedLogin)

		if counterResetDuration < timeSinceLastFailedLogin {
			log.Debug("Clearing failed login attempts for user: ", ma.UserName)
			ma.failedLoginAttempts = 0
		}
	}

	if ma.Locked {
		// If account is locked and the AccountLockoutDirection has elapsed unlock
		lockoutDuration := time.Duration(ma.as.AccountLockoutDuration) * time.Second
		timeSinceLockOut := time.Since(ma.lockOutStart)

		if lockoutDuration < timeSinceLockOut {
			log.Debug("Clearing lockout for user: ", ma.UserName)
			if err := ma.setLockOut(false); err != nil {
				log.Error(err)
				return err
			}
		}
	}

	return nil
}

// OperationAllowed will determine if this account has the required privileges to satisfy the provided operation
// privileges.
func (ma *ManagerAccount) OperationAllowed(privileges []OperationPrivilege) bool {
	return ma.Enabled && !ma.Locked && ma.GetRole().HasPrivilege(privileges)
}

// GetRole will returned this accounts role
func (ma *ManagerAccount) GetRole() *Role {
	role, ok := ma.as.Roles.get(ma.RoleId)
	if !ok {
		panic("Role does not exist")
	}

	return role
}

// Note: Roles can not be inserted or removed, but are allowed to be updated
type RoleCollection struct {
	Members map[string]*Role

	as *AccountService
}

func NewRoleCollection(as *AccountService) *RoleCollection {
	rc := &RoleCollection{}
	rc.Members = map[string]*Role{}
	rc.as = as
	return rc
}

// get will attempt to return the requested role
func (rc *RoleCollection) get(id string) (*Role, bool) {
	role, ok := rc.Members[id]
	return role, ok
}

// initFromRedis will initialize this collection (and its member roles) from data in redis
func (rc *RoleCollection) initFromRedis() *RoleCollection {
	log.Trace("Initializing Roles Collection")

	uri := "/redfish/v1/AccountService/Roles"
	resource, property := rc.as.rfd.GetResource(uri, "")
	if resource == nil {
		panic("Role Collection resource is nil")
	}
	if property != nil {
		panic("Role Collection URI returned a property")
	}

	r2s := dispatcher.Redis2Interface{
		RFD:      rc.as.rfd,
		Resource: resource,
		URI:      uri,
	}
	raw, err := r2s.BuildStruct(resource.Properties, uri)
	if err != nil {
		panic(err)
	}

	members := raw["Members"].([]interface{})

	for _, memberRaw := range members {
		member := memberRaw.(map[string]interface{})
		roleURI := member["@odata.id"].(string)
		role := NewRole(rc.as).initFromRedis(roleURI)

		// Add it to the collection
		rc.Members[role.RoleId] = role
	}

	log.Info("Loaded Roles Collection")

	// Return itself
	return rc
}

// Role is a partial representation of the redfish resource of the same name.
type Role struct {
	AssignedPrivileges []string
	RoleId             string

	as *AccountService
}

func NewRole(as *AccountService) *Role {
	r := &Role{}
	r.as = as
	return r
}

// initFromRedis will initialize this role from data in redis
func (r *Role) initFromRedis(uri string) *Role {
	log.WithFields(log.Fields{
		"uri": uri,
	}).Trace("Initializing Roll")

	resource, property := r.as.rfd.GetResource(uri, "")
	if resource == nil {
		log.Fatal("Manager Account resource is nil")
	}
	if property != nil {
		log.Fatal("Manager Account URI returned a property")
	}

	r2s := dispatcher.Redis2Interface{
		RFD:      r.as.rfd,
		Resource: resource,
		URI:      uri,
	}
	raw, err := r2s.BuildStruct(resource.Properties, uri)
	if err != nil {
		panic(err)
	}
	r.initFromStruct(raw)

	log.WithFields(log.Fields{
		"role": r.RoleId,
		"uri":  uri}).Debug("Loaded role")

	// Return itself
	return r
}

// initFromStruct will initialize this role from data in the provided map
func (r *Role) initFromStruct(raw map[string]interface{}) *Role {
	privileges := raw["AssignedPrivileges"].([]interface{})
	for _, privilegeRaw := range privileges {
		privilege := privilegeRaw.(string)
		r.AssignedPrivileges = append(r.AssignedPrivileges, privilege)
	}

	r.RoleId = raw["RoleId"].(string)
	// Return itself
	return r
}

// HasPrivilege will determine if this Role statifies the required operation privileges
func (r *Role) HasPrivilege(required []OperationPrivilege) bool {
	for _, op := range required {
		// TODO: Look into whether there are any cases where a Resource in the privilege registry has no
		// required privileges. If there are no cases then it could be safe to assume that a programing flaw allowed
		// there to be no required privileges.
		allowed := true
		for _, requiredPriv := range op.Privilege {
			found := false
			for _, p := range r.AssignedPrivileges {
				if requiredPriv == p {
					found = true
					break
				}
			}
			allowed = allowed && found
		}
		if allowed {
			return true
		}
	}

	// No matches
	return false
}

// struct2map will transform a Go structure into a map[string]interface{}. Only exported fields will be transformed, as
// reflection does not support accessing unexported fields.
// Note: This will only transform simple fields of the given structure. It will not decend into child strcutures
func struct2map(in interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	structValue := reflect.Indirect(reflect.ValueOf(in))
	if structValue.Kind() != reflect.Struct {
		return nil, errors.New("Argument is not a pointer")
	}

	structType := structValue.Type()

	for i := 0; i < structValue.NumField(); i++ {
		fieldType := structType.Field(i)
		isUnexported := fieldType.PkgPath != ""
		if isUnexported {
			log.Trace(fieldType.Name, " is unexported")
			continue
		}
		fieldValue := structValue.Field(i)

		simpleTypes := map[reflect.Kind]bool{
			reflect.Int64:  true, // TODO: For now only support int64
			reflect.Bool:   true,
			reflect.String: true,
		}

		if !simpleTypes[fieldValue.Kind()] {
			log.Trace("Skipping non-simple field: ", fieldType.Name)
			continue
		}

		result[fieldType.Name] = fieldValue.Interface()
	}

	return result, nil
}

// map2struct will set fields in a strucutre to the values contained in the provided map. This can be used to update
// fields in a existing structure as fields not present in the map will be overridden
// Only exported fields will be transformed, as reflection does not support accessing unexported fields.
// Note: This will only transform simple fields of the given structure. It will not decend into child strcutures
func map2struct(in map[string]interface{}, out interface{}) error {
	structValue := reflect.Indirect(reflect.ValueOf(out))
	if structValue.Kind() != reflect.Struct {
		return errors.New("Argument is not a pointer")
	}

	structType := structValue.Type()

	for name, value := range in {
		_, ok := structType.FieldByName(name)
		if !ok {
			log.Trace("Field does does not exist in struct: ", name)
			continue
		}

		fieldValue := structValue.FieldByName(name)

		simpleTypes := map[reflect.Kind]bool{
			reflect.Int64:  true, // TODO: For now only support int64
			reflect.Bool:   true,
			reflect.String: true,
		}

		mapValue := reflect.ValueOf(value)

		if !simpleTypes[mapValue.Kind()] {
			log.Trace("Skipping non-simple map field: ", name)
			continue
		}

		if mapValue.Kind() != fieldValue.Kind() {
			return errors.New("Map and field types do not match")
		}

		fieldValue.Set(mapValue)
	}

	return nil
}
