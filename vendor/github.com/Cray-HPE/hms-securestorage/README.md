# Secure Storage Package

## Overview

The *hms-securestorage* package is an adapter for access to
Hashicorp Vault.  It provides methods for:

* Adapter initialization,
* Key/value store, fetch, and delete
* K8s and Vault authorization setup
* Methods for more direct interaction with the Vault K/V store (typically not directly used by applications)

This package is used by the higher-level *hms-compcredentials* package to 
further abstract away the details of Vault interactions.


## Environment Variables

This package depends heavily on environment variables to provide various 
pieces of configuration information needed by the Hashicorp Vault API.  
Fortunately, the defaults suffice for nearly all of them.   The variables 
are listed below, with explanations of the few that are needed by most 
applications:


* **VAULT_ADDR** - URL of Hashicorp Vault service, e.g. http://cray-vault.vault:8200
* **VAULT_SKIP_VERIFY** -- Typically set to 'true'
* **VAULT_TOKEN** -- Specify key space.  HMS services set this to 'hms'

The following are typically set according to defaults generated by a system's
configuration mechanism, including k8s sealed secrets, and are thus available
to and already set in a microservice's environment.

* **CRAY_VAULT_JWT_FILE** -- Specifies the path of a file containing an access token used for k8s authN/authZ.  The default is */var/run/secrets/kubernetes.io/serviceaccount/token*.
* **CRAY_VAULT_ROLE_FILE** -- Specifies the path of a file containing a name space used by k8s.  Default is */var/run/secrets/kubernetes.io/serviceaccount/namespace*.
* **CRAY_VAULT_AUTH_PATH** -- Used for k8s access.  Default will suffice for production deployments.  Default is *auth/kubernetes/login*.  In testing environments it can be set to *auth/token/create*.


The following are typically not set in HMS services; default values are safe:

* VAULT_AGENT_ADDR
* VAULT_CACERT
* VAULT_CAPATH
* VAULT_CLIENT_CERT
* VAULT_CLIENT_KEY
* VAULT_CLIENT_TIMEOUT
* VAULT_NAMESPACE
* VAULT_TLS_SERVER_NAME
* VAULT_WRAP_TTL
* VAULT_MAX_RETRIES
* VAULT_MFA
* VAULT_RATE_LIMIT


## Adapter Initialization

Typical usage by applications begins by creating and initializing a Vault 
adapter, assuming the environment variables specified above have been set:

```
...
	ss,err := securestorage.NewVaultAdapter("secret")
	if (err != nil) {
		log.Printf("Unable to create Vault adapter: %v",err)
	}
...
```

The string "secret" is a path within the overall key space used by the 
application, and is the typical value used by HMS microservices.

The returned handle can then be used for storing/fetching/deleting key/value 
entries.

## Most-Used Methods

The following methods are the ones most used by applications.

```
// Create a new SecureStorage interface that uses Vault. This connects an
// application to Vault.
func NewVaultAdapter(basePath string) (SecureStorage, error)


// Write a value to Vault at the location specified by 'key'. This function
// prepends the basePath. Retries are implemented for token renewal.  The
// specified value should not be marshalled or encoded in any way.
func (ss *VaultAdapter) Store(key string, value interface{}) error


// Read a value from Vault at the location specified by key. This function
// prepends the basePath. Retries are automatically done for token renewal.
// Note that the resulting value is unmarshalled and returned in the 
// 'output' argument.
func (ss *VaultAdapter) Lookup(key string, output interface{}) error {


// Get a list of keys that exsist in Vault at the path specified by keyPath.
// This function prepends the basePath. Retries are automatically done for
// token renewal.
func (ss *VaultAdapter) LookupKeys(keyPath string) ([]string, error)


// Remove a value from Vault at the location specified by key. This function
// prepends the basePath. Retries are implemented for token renewal.
func (ss *VaultAdapter) Delete(key string) error
```


## Lower Level Mechanisms

In addition to the above typically-used methods there are also lower-level
methods that can be directly used by applications.  **Note that these lower-level
mechanisms are used by the higher-level ones outlined above so there is
typically no need to use them.**


### K8s Authentication Support

The following are used for authN support for Kubernetes.

```
// AuthConfig struct for vault k8s authentication
type AuthConfig struct {
	JWTFile  string
	RoleFile string
	Path     string
	jwt      string
	role     string
}


// ReadEnvironment Update an AuthConfig object with environment variables
//   CRAY_VAULT_JWT_FILE
//   CRAY_VAULT_ROLE_FILE
//   CRAY_VAULT_AUTH_PATH
func (authConfig *AuthConfig) ReadEnvironment() error


// LoadJWT save contents of JWT file to the AuthConfig jwt field.  This is
// used for manual JWT token refresh.
func (authConfig *AuthConfig) LoadJWT() error


// Manually load contents of RoleFile into the role field
func (authConfig *AuthConfig) LoadRole() error


// Getter method for auth path key
func (authConfig *AuthConfig) GetAuthPath() string


// Generates the args required for generating an auth token
func (authConfig *AuthConfig) GetAuthArgs() map[string]interface{}
```

### Low-Level Vault Access

This package provides a mechanism for a more direct access to the Vault API.
This is generally not used by applications; using it will require code changes
if Vault is ever swapped out for another secure storage system.

These methods and data structures use the Vault 'api' object directly.

```
///////////////////////////////////////////////////////////////////////////////
// Vault API interface - This interface wraps only a subset of functions for
// api.Client so as to reduce the amount of functions that need to be mocked
// for unit testing.
///////////////////////////////////////////////////////////////////////////////
type VaultApi interface {
	Read(path string) (*api.Secret, error)
	Write(path string, data map[string]interface{}) (*api.Secret, error)
	Delete(path string) (*api.Secret, error)
	List(path string) (*api.Secret, error)
	SetToken(t string)
}

type RealVaultApi struct {
	Client *api.Client
}


// Create a low-level Vault API object
func NewRealVaultApi(client *api.Client) VaultApi


// Apply a JWT token to the low-level Vault
func (v *RealVaultApi) SetToken(t string)


// Read a K/V from low-level Vault.  Returns a secret record containing the
// key's value.
func (v *RealVaultApi) Read(path string) (*api.Secret, error)


// Write a K/V to low-level Vault.  Returns the secret record modified by
// the write operation.
func (v *RealVaultApi) Write(path string, data map[string]interface{}) (*api.Secret, error)


// Delete a key in low-level Vault.  
func (v *RealVaultApi) Delete(path string) (*api.Secret, error)


// List all keys in the specified key space.  Returns secret record 
// containing all keys in the space.
func (v *RealVaultApi) List(path string) (*api.Secret, error)
```

## Typical Usage

Following is an example of the *hms-securestorage* package.  Note that this
example is mostly centered around the *hms-compcredentials* package, as that
package is the one predominantly used in HMS services.

```
...
import (
    sstorage "github.com/Cray-HPE/hms-securestorage"
    compcreds "github.com/Cray-HPE/hms-compcredentials"
)
...

    // Create the Vault adapter and connect to Vault

    ss, err := sstorage.NewVaultAdapter("secret")
    if err != nil {
        return fmt.Errorf("Error: %v", err)
    }

    // Initialize the CompCredStore struct with the Vault adapter.
	// Use the 'hms-creds' key space

    ccs := compcreds.NewCompCredStore("hms-creds", ss)

    // Create a new set of credentials for a component.

    compCred := compcreds.CompCredentials{
        Xname: "x0c0s21b0"
        URL: "10.4.0.8/redfish/v1/UpdateService"
        Username: "test"
        Password: "123"
    }

    // Store the credentials in the CompCredStore (backed by Vault).

    err = ccs.StoreCompCred(compCred)
    if err != nil {
        return fmt.Errorf("Error: %v", err)
        
    }

    // Read the credentials for a component from the CompCredStore
    // (backed by Vault).

    var ccred CompCredentials
    ccred, err = ccs.GetCompCred(compCred.Xname)
    if err != nil {
        return fmt.Errorf("Error: %v", err)
    }

    log.Printf("%v", ccred)
...
```
