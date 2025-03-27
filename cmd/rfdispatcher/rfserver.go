/*
 *
 *  MIT License
 *
 *  (C) Copyright 2018-2022,2025 Hewlett Packard Enterprise Development LP
 *
 *  Permission is hereby granted, free of charge, to any person obtaining a
 *  copy of this software and associated documentation files (the "Software"),
 *  to deal in the Software without restriction, including without limitation
 *  the rights to use, copy, modify, merge, publish, distribute, sublicense,
 *  and/or sell copies of the Software, and to permit persons to whom the
 *  Software is furnished to do so, subject to the following conditions:
 *
 *  The above copyright notice and this permission notice shall be included
 *  in all copies or substantial portions of the Software.
 *
 *  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 *  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 *  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 *  THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
 *  OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
 *  ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
 *  OTHER DEALINGS IN THE SOFTWARE.
 *
 */

/*
 * A Redfish dispatch server based on DMTF schema files
 *
 */
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/Cray-HPE/hms-redfish-translation-service/internal/logger"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/accountservice"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/dispatcher"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/hooks"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/jsonschemas"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfschema"

	// Used for Profiling using the go pprof tool
	_ "net/http/pprof"

	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
)

var (
	restSrvHTTP  *http.Server = nil
	restSrvHTTPS *http.Server = nil
	ticker       *time.Ticker
	waitGroup    sync.WaitGroup

	handleRestRequests	 chan	// Used to signal the REST server to start handling requests

	running = true

	ctx context.Context

	server *redfishServer
)

type redfishServer struct {
	// Server properties
	address   string
	httpPort  int
	httpsPort int

	rfd *dispatcher.RedfishDispatcher

	accountService        *accountservice.AccountService
	privilegeRegistry     *accountservice.PrivilegeRegistry
	jsonSchemasCollection *jsonschemas.JsonSchemaFileCollection

	beforeHookIns map[string][]hooks.HookIn
	afterHookIns  map[string][]hooks.HookIn
}

func (rs *redfishServer) redis() *redis.Client {
	return rs.rfd.Redis()
}

// httpRequest contains
type httpRequest struct {
	httpOperation string
	account       *accountservice.ManagerAccount
	uri           string
	resource      *rfschema.Resource
	property      *rfschema.Property
	xname         string

	requestBody []byte
}

// addHookIn will register the hook with the server for a particular HTTP method
func (rs *redfishServer) addHookIn(method string, hook hooks.HookIn) {
	if hook.BeforeCB != nil {
		rs.beforeHookIns[method] = append(rs.beforeHookIns[method], hook)
	}

	if hook.AfterCB != nil {
		rs.afterHookIns[method] = append(rs.afterHookIns[method], hook)
	}
}

// findBeforeHookIns will find all applicable hooks for the provided HTTP method and URI that need to fun before
// the operation is performed
func (rs *redfishServer) findBeforeHookIns(method, uri string) []hooks.HookIn {
	result := []hooks.HookIn{}
	for _, hook := range rs.beforeHookIns[method] {
		if hook.URIRegex.MatchString(uri) {
			result = append(result, hook)
		}
	}
	return result
}

// findAfterHookIns will find all applicable hooks for the provided HTTP method and URI that need to fun before
// the operation is performed
func (rs *redfishServer) findAfterHookIns(method, uri string) []hooks.HookIn {
	result := []hooks.HookIn{}
	for _, hook := range rs.afterHookIns[method] {
		if hook.URIRegex.MatchString(uri) {
			result = append(result, hook)
		}
	}
	return result
}

// runBeforeHookIns will execute all before hooks that match the provided HTTP method and URI.
// If an error occurs in a hook then either a internal sever error is returned, or hooks error is returned to the
// client.
func (rs *redfishServer) runBeforeHookIns(method string, event hooks.Event, ctx, request map[string]interface{}) ([]byte, int) {
	for _, hook := range rs.findBeforeHookIns(method, event.URI) {
		hookResponse, hookStatus, err := hook.BeforeCB(event, ctx, request)
		// If the hook returns an non-nil hook response with a error display the hooks response, otherwise
		// send a internal server error
		if err != nil {
			if hookResponse == nil {
				response, status := rs.internalServerError(event.URI, err)
				return response, status
			}
			return hookResponse, hookStatus
		}
	}

	return nil, 0
}

// runBeforeHookIns will execute all after hooks that match the provided HTTP method and URI.
// If an error occurs in a hook then either a internal sever error is returned, or hooks error is returned to the
// client.
func (rs *redfishServer) runAfterHookIns(method string, event hooks.Event, ctx, request map[string]interface{}) ([]byte, int) {
	for _, hook := range rs.findAfterHookIns(method, event.URI) {
		hookResponse, hookStatus, err := hook.AfterCB(event, ctx, request)
		// If the hook returns an non-nil hook response with a error display the hooks response, otherwise
		// send a internal server error
		if err != nil {
			if hookResponse == nil {
				response, status := rs.internalServerError(event.URI, err)
				return response, status
			}
			return hookResponse, hookStatus
		}
	}

	return nil, 0
}

func doRest() {
	httpPortStr := fmt.Sprintf("%d", server.httpPort)
	restSrvHTTP = &http.Server{Addr: ":" + httpPortStr}
	httpsPortStr := fmt.Sprintf("%d", server.httpsPort)
	restSrvHTTPS = &http.Server{Addr: ":" + httpsPortStr}

	httpsCert, httpsCertExists := os.LookupEnv("HTTPS_CERT")
	httpsKey, httpsKeyExists := os.LookupEnv("HTTPS_KEY")

	http.HandleFunc(server.address, server.handleRequest)

	// Used to serve schema files for the JSON Schema Service.
	// Since they are not apart of the Redfish Schema (the schema files themselves not the JsonSchemaFile resource).
	http.HandleFunc("/redfish/v1/JsonSchemas/Files/", server.jsonSchemasCollection.ServeSchemaFile)

	// Health function for Kubernetes liveness/readiness.
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Beef up this health check.
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	if httpsCertExists && httpsKeyExists {
		// If we're going to serve HTTPs we need to add to the wait group.
		waitGroup.Add(1)
	}

	go func() {
		var err error

		if httpsCertExists && httpsKeyExists {
			defer waitGroup.Done()

			// Only if we're given a certificate should we enable HTTPS
			log.Info("Enabling HTTPs")
			err = server.rfd.CertificateService.LoadDefaultCert(httpsCert, httpsKey)
			if err != nil {
				log.WithFields(log.Fields{
					"httpsCert": httpsCert,
					"httpsKey":  httpsKey,
					"err":       err,
				}).Fatal("Unable to load Default HTTPS Cert")
			}

			err = server.rfd.CertificateService.WatchDefaultCertificate(httpsCert, httpsKey)
			if err != nil {
				log.WithFields(log.Fields{
					"httpsCert": httpsCert,
					"httpsKey":  httpsKey,
					"err":       err,
				}).Fatal("Unable to start the default HTTPS Cert watcher")
			}

			// When a HTTPS connection is established we want to select the correct cert based off of the
			// hostname of the request. This is called SNI
			tlsConfig := &tls.Config{
				GetCertificate: server.rfd.CertificateService.ReturnCert,
			}

			tlsListener, err := tls.Listen("tcp", restSrvHTTPS.Addr, tlsConfig)
			if err != nil {
				log.WithFields(log.Fields{
					"httpsPortStr": httpsPortStr,
					"err":          err,
				}).Fatal("Unable to setup TLS Listener")
			}

			err = restSrvHTTPS.Serve(tlsListener)
			if err != nil {
				if err.Error() == "http: Server closed" {
					log.Info("REST HTTPS server shutdown")
				} else {
					log.WithField("err", err).Panic("Unable to start REST HTTPS server")
				}
			}
		}
	}()
	go func() {
		defer waitGroup.Done()

		var err error

		// Wait until everything is initialized before processing requests
		<-handleRestRequests

		log.Info("Beginning to handle REST requests...")

		// Always enable HTTP
		err = restSrvHTTP.ListenAndServe()

		if err != nil {
			if err.Error() == "http: Server closed" {
				log.Info("REST HTTP server shutdown")
			} else {
				log.WithField("err", err).Panic("Unable to start REST HTTP server")
			}
		}
	}()

	log.WithField("httpPort", httpPortStr).Info("Listing for incoming HTTP requests")
	if httpsCertExists && httpsKeyExists {
		log.WithField("httpsPort", httpsPortStr).Info("Listing for incoming HTTPS requests")
	}
}

func main() {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())
	handleRestRequests = make(chan struct{})

	server = newRedfishServer(ctx)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-c
		running = false

		// Cancel the context to cancel any in progress HTTP requests.
		cancel()

		if restSrvHTTP != nil {
			if err := restSrvHTTP.Shutdown(ctx); err != nil {
				log.WithField("err", err).Panic("Unable to stop HTTP REST server!")
			}
		}
		if restSrvHTTPS != nil {
			if err := restSrvHTTPS.Shutdown(ctx); err != nil {
				log.WithField("err", err).Panic("Unable to stop HTTPS REST server!")
			}
		}
		if ticker != nil {
			ticker.Stop()
		}
	}()

	// Command line flags
	flag.Parse()

	log.Info("Redfish Translation Service starting...")

	waitGroup.Add(1)
	go doRest()

	if len(server.rfd.BackendHelpers) > 0 {

		// Now run manually the first version of the periodic for any backend helper and setup the ticker.
		server.rfd.RunPeriodic()

		// Signal the rest server that it can start handling requests
		close(handleRestRequests)

		var periodSeconds int
		periodSecondsString, ok := os.LookupEnv("PERIODIC_SLEEP")
		if !ok {
			periodSeconds = 60
		} else {
			var err error
			periodSeconds, err = strconv.Atoi(periodSecondsString)
			if err != nil {
				log.Fatal("PERIODIC_SLEEP value not integer")
			}
		}
		log.WithField("periodSeconds", periodSeconds).Debug("Running backend helper period function")

		ticker = time.NewTicker(time.Duration(periodSeconds) * time.Second)
		go func() {
			for range ticker.C {
				log.Trace("Running ticker")
				server.rfd.RunPeriodic()
				log.Trace("Finished ticker")
			}
		}()
	}

	waitGroup.Wait()

	log.Info("Redfish Translation Service shutting down")
}

func newRedfishServer(ctx context.Context) *redfishServer {
	server := &redfishServer{}
	server.address = "/"
	server.httpPort = 8082
	server.httpsPort = 8083

	server.beforeHookIns = map[string][]hooks.HookIn{}
	server.afterHookIns = map[string][]hooks.HookIn{}

	// Note: See dispatcher.NewDispatcher() for other hard coded options
	server.rfd = dispatcher.NewDispatcher(ctx)
	server.initAccountService()
	server.initJSONSchemaService()

	return server
}

// initAccountService will initialize the account service for user, and register any hooks with the server
// The Path to the privilege registry is set through a environment variables
func (rs *redfishServer) initAccountService() {
	rs.accountService = accountservice.NewAccountService(rs.rfd)
	rs.accountService.InitFromRedis()
	log.Info("Account Service ready")

	privilegeRegistryVersion, ok := os.LookupEnv("PRIVILEGE_REGISTRY_VERSION")
	if !ok {
		log.Panic("This requires the environment variable PRIVILEGE_REGISTRY_VERSION to be set")
	}
	privilegeRegistryPath := filepath.Join(rs.rfd.SchemaConfig.ContribDirPrefix,
		"contrib/DMTF/DSP8011-Redfish_Standard_Registries/Redfish_"+privilegeRegistryVersion+"_PrivilegeRegistry.json")
	privilegeRegistry, err := accountservice.NewPrivilegeRegistry(privilegeRegistryPath)
	if err != nil {
		panic(err)
	}
	rs.privilegeRegistry = privilegeRegistry
	log.Info("Privilege Registry ready")

	// Start periodic task
	go rs.accountService.RunPeriodic()

	// NOTE: Roles currently can not be updated, as the rfd.RedisPatch method does not support JSON arrays, and
	// the only writable field for an role are the privileges which are represented as arrays.

	// Get Hooks
	// None

	// Post Hooks
	rs.addHookIn(http.MethodPost, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/AccountService\\/Accounts$"),
		BeforeCB: rs.accountService.Accounts.AddAccountBefore,
		AfterCB:  rs.accountService.Accounts.AddAccountAfter,
	})

	// Patch Hooks
	// Account Service
	rs.addHookIn(http.MethodPatch, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/AccountService$"),
		AfterCB:  rs.accountService.Update,
	})
	// Role
	// Account
	rs.addHookIn(http.MethodPatch, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/AccountService\\/Accounts\\/[A-Za-z0-9]+$"),
		BeforeCB: rs.accountService.Accounts.UpdateAccountBefore,
		AfterCB:  rs.accountService.Accounts.UpdateAccountAfter,
	})

	// Put Hooks
	// Account Service
	rs.addHookIn(http.MethodPut, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/AccountService$"),
		AfterCB:  rs.accountService.Update,
	})

	// Role
	// Account
	rs.addHookIn(http.MethodPut, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/AccountService\\/Accounts\\/[A-Za-z0-9]+$"),
		BeforeCB: rs.accountService.Accounts.UpdateAccountBefore,
		AfterCB:  rs.accountService.Accounts.UpdateAccountAfter,
	})

	// Delete Hooks
	rs.addHookIn(http.MethodDelete, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/AccountService\\/Accounts\\/[A-Za-z0-9]+$"),
		AfterCB:  rs.accountService.Accounts.RemoveAccount,
	})
}

func (rs *redfishServer) initJSONSchemaService() {
	rs.jsonSchemasCollection = jsonschemas.NewJsonSchemaFileCollection(rs.rfd.SchemaTree())

	rs.rfd.RegisterCollectionCB(func(collection *rfschema.Resource, collectionPath string, instance string) bool {
		if collection.Name != "JsonSchemaFileCollection" && collectionPath != "/redfish/v1/JsonSchemas" {
			return false
		}

		log.Trace("Running JSONSchema collection callback for: ", instance)
		return rs.jsonSchemasCollection.Exists(instance + ".json")
	})

	rs.addHookIn(http.MethodGet, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/JsonSchemas$"),
		AfterCB:  rs.jsonSchemasCollection.HandleCollection,
	})

	// Need to take over the GET request before the rfserver attempts to query redis
	rs.addHookIn(http.MethodGet, hooks.HookIn{
		URIRegex: regexp.MustCompile("^\\/redfish\\/v1\\/JsonSchemas\\/[A-Za-z0-9._]+$"),
		BeforeCB: rs.jsonSchemasCollection.HandleResource,
	})

	log.Info("JSON Schema Service ready")
}

/* Debug-only function for printing JSON payloads */
func printStructAsJSON(payload map[string]interface{}) {
	jsonPayload, err := json.MarshalIndent(payload, "", "    ")
	if err == nil {
		log.Debug(string(jsonPayload))
	} else {
		log.Error("Failed to convert to JSON")
	}
}

/* Debug function for displaying validation results from an Object Report */
func printObjectResport(or ObjectReport) {
	or.log("")
}

/* Helper function to build a resource missing error at uri message */
func (rs *redfishServer) actionParameterMissing(action string, parameter string) ([]byte, int) {
	msg, _ := rs.rfd.GetRedfishMessageString("Base", "ActionParameterMissing", action, parameter)
	log.WithFields(log.Fields{
		"msg": msg,
	}).Warning("Action parameter missing")
	return []byte(msg), http.StatusBadRequest
}

/* Helper function to build a resource missing error at uri message */
func (rs *redfishServer) resourceMissing(uri string) ([]byte, int) {
	msg, _ := rs.rfd.GetRedfishMessageString("Base", "ResourceMissingAtURI", uri)
	log.WithFields(log.Fields{
		"uri": uri,
		"msg": msg,
	}).Warning("Resource missing")
	return []byte(msg), http.StatusNotFound
}

func (rs *redfishServer) internalServerError(uri string, err error) ([]byte, int) {
	msg, _ := rs.rfd.GetRedfishMessageString("Base", "InternalError")
	log.WithFields(log.Fields{
		"uri": uri,
		"msg": msg,
		"err": err,
	}).Error("Internal Server Error")
	return []byte(msg), http.StatusInternalServerError
}

/* Top-level handler for all HTTP request */
func (rs *redfishServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			log.Error("Error in handler:", err)
			log.Error("Stack trace: ", string(debug.Stack()))
			msg, _ := rs.rfd.GetRedfishMessageString("Base", "InternalError")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(msg))
		}
	}()

	log.WithFields(log.Fields{
		"header":     r.Header,
		"host":       r.Host,
		"r.URL.Path": r.URL.Path,
	}).Debug("Incoming request")

	var responseHTTPCode int
	var responseBody []byte
	headers := map[string]string{}
	httpRequestBody := new(bytes.Buffer)

	// Use the host information to get the Xname.
	hostURLParts := strings.Split(r.Host, ":")
	host := hostURLParts[0]

	// Strip off '-rts' prefix on hostname if present.
	xname := strings.TrimSuffix(host, "-rts")

	// TODO: Update to return a Resource XOR Property
	// For now no POST validation, Also Property.Name would be nice
	resource, property := rs.rfd.GetResource(r.URL.Path, xname)

	// Perform Authentication here
	username, password, ok := r.BasicAuth()
	if !ok {
		log.Debug("Request is missing Basic Auth")
	}

	account, err := rs.accountService.AuthenticateAccount(username, password)
	if err != nil {
		log.WithFields(log.Fields{"username": username, "xname": xname, "err": err}).Error("Could not authenticate account for username")
		responseHTTPCode = http.StatusUnauthorized
		// TODO this seems a little hacky
		w.WriteHeader(responseHTTPCode)
		w.Write(responseBody)
		return
	}

	httpRequestBody.ReadFrom(r.Body)
	request := httpRequest{
		account:  account,
		uri:      r.URL.Path,
		resource: resource,
		property: property,
		xname:    xname,

		httpOperation: r.Method,
		requestBody:   httpRequestBody.Bytes(),
	}

	r2s := dispatcher.Redis2Interface{
		RFD:      rs.rfd,
		Resource: resource,
		URI:      request.uri,
		XName:    request.xname,
	}

	switch r.Method {
	case http.MethodPost:
		responseBody, headers, responseHTTPCode = rs.handlePostRequest(request)
	case http.MethodGet:
		responseBody, responseHTTPCode = rs.handleGetRequest(request, &r2s)
	case http.MethodPatch:
		responseBody, responseHTTPCode = rs.handlePatchRequest(request, &r2s)
	case http.MethodDelete:
		responseBody, responseHTTPCode = rs.handleDeleteRequest(request, &r2s)
	case http.MethodPut:
		httpRequestBody.ReadFrom(r.Body)
		responseBody, responseHTTPCode = rs.handlePutRequest(request, &r2s)
	default:
		/* The following are optional per Redfish spec DSP0266_1.0:
		 * http.MethodPut, http.MethodHead, http.MethodOptions
		 * "The response shall include an Allow header which provides a
		 * list of methods that are supported by the resource identified
		 * by the Request-uri." */
		responseHTTPCode = http.StatusMethodNotAllowed
		responseBody = []byte("Message: CustomHTTP->StatusMethodNotAllowed\n")
	}

	// Need to set headers before writing them
	if responseBody != nil {
		w.Header().Set("Content-Type", "application/json")
	}

	// Handler defined headers
	for header, value := range headers {
		w.Header().Set(header, value)
	}

	w.WriteHeader(responseHTTPCode)
	w.Write(responseBody)
}

/* This handles all Redfish collections (instance and resource) */
func (rs *redfishServer) handleGetRequest(r httpRequest, r2s *dispatcher.Redis2Interface) ([]byte, int) {
	resource := r.resource
	property := r.property
	uri := r.uri

	if resource == nil {
		return rs.resourceMissing(uri)
	}

	if property != nil {
		// Get method only supports resources, not properties of resources
		log.WithFields(log.Fields{
			"uri":      uri,
			"resource": resource.Name,
			"property": property,
		}).Error("Found property at URI durring GET request")
		return rs.resourceMissing(uri)
	}

	// Check for privilege
	privileges := rs.privilegeRegistry.RequiredPrivileges(r.httpOperation, "", r.resource.Name, "")
	if !r.account.OperationAllowed(privileges) {
		log.WithFields(log.Fields{
			"username":      r.account.UserName,
			"uri":           r.uri,
			"httpOperation": r.httpOperation,
		}).Warning("User does not have required privileges")
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "InsufficientPrivilege")
		return []byte(msg), http.StatusUnauthorized
	}

	// Run any before hooks
	event := hooks.Event{
		HTTPOperation: r.httpOperation,
		URI:           r.uri,
		Resource:      r.resource,
		Property:      r.property,
	}
	ctx := map[string]interface{}{}
	hookResponse, hookStatus := rs.runBeforeHookIns(http.MethodGet, event, ctx, nil)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	// responsePayload := map[string]interface{}{}
	var err error

	// Remove trailing / if present
	uri = strings.TrimSuffix(uri, "/")

	responsePayload, err := r2s.BuildStruct(resource.Properties, uri)
	if err != nil {
		err = fmt.Errorf("unable to build struct: %s", err.Error())
		log.WithField("resource", resource).Error("Unable to build struct")
		return nil, http.StatusInternalServerError
	}

	// Run after before hooks
	hookResponse, hookStatus = rs.runAfterHookIns(http.MethodGet, event, ctx, responsePayload)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	jsonPayload, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, http.StatusInternalServerError
	}

	// DEBUG: Pretty print from server
	printStructAsJSON(responsePayload)

	return jsonPayload, http.StatusOK
}

/* Handles all redfish POST request. Will translate to object insertion and
 * and actions. */
func (rs *redfishServer) handlePostRequest(r httpRequest) ([]byte, map[string]string, int) {
	if r.resource == nil {
		response, status := rs.resourceMissing(r.uri)
		return response, nil, status
	}

	// Check for privilege
	privileges := rs.privilegeRegistry.RequiredPrivileges(r.httpOperation, "", r.resource.Name, "")
	if !r.account.OperationAllowed(privileges) {
		log.WithFields(log.Fields{
			"username":      r.account.UserName,
			"uri":           r.uri,
			"httpOperation": r.httpOperation,
		}).Warning("User does not have required privileges")
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "InsufficientPrivilege")
		return []byte(msg), nil, http.StatusUnauthorized
	}

	// Check to see if this post is for an action
	if r.resource != nil && r.property != nil && r.property.Type == rfschema.PropAction {
		response, err := rs.rfd.HandleAction(r.property, r.xname+r.uri, r.requestBody, r.xname)
		if err == nil {
			if response == nil {
				// No body, 204 No Content
				return nil, nil, http.StatusNoContent
			} else {
				headers := make(map[string]string)
				headers["Content-Type"] = "application/json"
				return response, headers, http.StatusOK
			}
		}

		response, status := rs.internalServerError(r.uri, fmt.Errorf("Action failed: %w", err))
		return response, nil, status
	}

	// POST requests are submitted to Resource Collections in which the new resource belongs to.
	if r.resource.IsCollection {
		// Object creation - Post request
		return rs.handleResourceCreation(r)
	}

	return []byte("Message: CustomHTTP->StatusMethodNotAllowed\n"), nil, http.StatusMethodNotAllowed
}

/* Helper function to handle the creation of a new resource in a collection.
 * TODO: Does not support posting to {collection uri}/Members, only on the collection's URI */
func (rs *redfishServer) handleResourceCreation(r httpRequest) ([]byte, map[string]string, int) {
	collection := r.resource
	resource := collection.CollectionOf
	// TODO Support post on {collection uri}/Members
	// TODO Do not require @odata annotations like @odata.id in the payload, because this is determined by the
	// collection's URI and the Id supploed in the payload.

	// Is the post method allowed on the resource's collection
	if !collection.Insertable {
		return []byte("Message: CustomHTTP->ResourceNotInsertable\n"), nil, http.StatusBadRequest
	}

	var postBodyMap map[string]interface{}
	if err := json.Unmarshal(r.requestBody, &postBodyMap); err != nil {
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "MalformedJSON")
		return []byte(msg), nil, http.StatusBadRequest
	}
	if len(postBodyMap) == 0 {
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "EmptyJSON")
		return []byte(msg), nil, http.StatusBadRequest
	}

	// Validate input
	report := checkResource(resource, postBodyMap)
	printObjectResport(report)

	// Make sure all required properties are present
	if len(report.MissingRequired) > 0 {
		log.Debug("Missing required property ", strings.Join(report.getMissingRequired(), ", "))

		// Return an array of redfish messages, and the message for each missing property
		messages := []interface{}{}
		for missing := range report.MissingRequired {
			msg, err := rs.rfd.GetRedfishMessage("Base", "PropertyMissing", missing)
			if err != nil {
				panic(err)
			}
			messages = append(messages, msg)
		}

		payload, err := json.Marshal(messages)
		if err != nil {
			panic(err)
		}
		return payload, nil, http.StatusBadRequest
	}

	// Make sure all required on create properties are present
	if len(report.MissingRequiredOnCreate) > 0 {
		log.Debug("Missing required on create property", strings.Join(report.getMissingRequiredOnCreate(), ", "))
		// Return an array of redfish messages, and the message for each missing property
		messages := []interface{}{}
		for missing := range report.MissingRequired {
			msg, err := rs.rfd.GetRedfishMessage("Base", "CreateFailedMissingReqProperties", missing)
			if err != nil {
				panic(err)
			}
			messages = append(messages, msg)
		}

		payload, err := json.Marshal(messages)
		if err != nil {
			panic(err)
		}
		return payload, nil, http.StatusBadRequest
	}

	// The Name/Id of this new resource exists in one of two places in the posted resource:
	// - Id
	// - MemberId
	var id string
	if idRaw, ok := postBodyMap["Id"]; ok {
		id = idRaw.(string)
	}
	if idRaw, ok := postBodyMap["MemberId"]; ok {
		id = idRaw.(string)
	}

	if id == "" {
		panic("Resource ID not present")
	}

	if _, present := collection.Properties[id]; present {
		// The name of the instance shares the name with one of the collection's properties, this is not a valid
		// instance name
		log.Warn("The instance name: ", id, " is invalid for the collection ", r.uri)
		return []byte("Message: CustomHTTP->ResourceNameIsInvalid\n"), nil, http.StatusBadRequest
	}

	keySpace := r.uri + "/" + id
	exists, err := rs.redis().Exists(keySpace).Result()
	if err != nil {
		panic(err)
	}
	if exists != 0 {
		log.Warn("Resource already exists at ", keySpace)
		// TODO Not sure if right status code
		return []byte("Message: CustomHTTP->ResourceAlreadyExists\n"), nil, http.StatusConflict
	}

	// Run any before hooks
	event := hooks.Event{
		HTTPOperation: r.httpOperation,
		URI:           r.uri,
		Resource:      r.resource,
		Property:      r.property,
	}
	ctx := map[string]interface{}{}
	hookResponse, hookStatus := rs.runBeforeHookIns(http.MethodPost, event, ctx, postBodyMap)
	if hookResponse != nil {
		return hookResponse, nil, hookStatus
	}

	log.Info("Creating new Resource at ", keySpace)

	data, err := rs.rfd.Interface2Redis([]string{}, resource.Properties, postBodyMap, nil)
	if err != nil {
		panic(err)
	}

	// Ignore the content of the following odata annotations, as they are generated.
	// When redisInsert encounters a nil value no value will be added in redis, but the object set in redis
	// will specify what that it is a member. Since there is no value it will be dispatched
	data["@odata.id"] = nil
	data["@odata.type"] = nil
	data["@odata.context"] = nil

	err = rs.rfd.RedisInsert(data, keySpace)
	if err != nil {
		panic(err)
	}

	// Append new resource to the collection's Members field
	// Find the next available array id
	indexesInUse, err := rs.redis().LRange(r.uri+"/"+"Members", 0, -1).Result()
	if err != nil {
		panic(err)
	}

	offset := 0
	for _, iRaw := range indexesInUse {
		i64, err := strconv.ParseInt(iRaw, 10, 64)
		i := int(i64)
		if err != nil {
			panic(err)
		}

		if offset < i {
			offset = i
		}
	}
	offset++

	// Insert id into the collection's members array
	objIndex := offset
	log.Trace("RPUSH ", r.uri+"/"+"Members ", objIndex)
	err = rs.redis().RPush(r.uri+"/"+"Members", objIndex).Err()
	if err != nil {
		panic(err)
	}

	// Create set named '{uri}/Members' with '@odata.id' as an element
	// Create key `{uri}/id/@odata.id` with a link to the name of the new collection member
	resourceURI := r.uri + "/" + id
	linkKey := r.uri + "/" + "Members" + "/" + fmt.Sprint(offset)
	link := map[string]interface{}{}
	link["@odata.id"] = resourceURI
	rs.rfd.RedisInsert(link, linkKey)

	// JSON representation of the newly created resource after the request has been applied.
	r2s := dispatcher.Redis2Interface{
		RFD:      rs.rfd,
		Resource: resource,
		URI:      resourceURI,
		XName:    r.xname,
	}
	responsePayload, err := r2s.BuildStruct(resource.Properties, resourceURI)
	if err != nil {
		log.WithField("request", r).Error("Unable to build struct")
	}
	jsonPayload, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, nil, http.StatusInternalServerError
	}

	// Run any after hooks
	hookResponse, hookStatus = rs.runAfterHookIns(http.MethodPost, event, ctx, responsePayload)
	if hookResponse != nil {
		return hookResponse, nil, hookStatus
	}

	// DEBUG: Pretty print from server
	printStructAsJSON(responsePayload)

	// Set location header
	headers := map[string]string{}
	headers["Location"] = resourceURI

	return jsonPayload, headers, http.StatusCreated
}

/* handleDeleteRequest will delete a resource. */
func (rs *redfishServer) handleDeleteRequest(r httpRequest, r2s *dispatcher.Redis2Interface) ([]byte, int) {
	// Delete a resource
	if r.resource == nil {
		return rs.resourceMissing(r.uri)
	}

	// Check Privilege
	privileges := rs.privilegeRegistry.RequiredPrivileges(r.httpOperation, "", r.resource.Name, "")
	if !r.account.OperationAllowed(privileges) {
		log.WithFields(log.Fields{
			"username":      r.account.UserName,
			"uri":           r.uri,
			"httpOperation": r.httpOperation,
		}).Warning("User does not have required privileges")
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "InsufficientPrivilege")
		return []byte(msg), http.StatusUnauthorized
	}

	// If the resource is a collection return Status not allowed (405)
	if r.resource.IsCollection {
		log.Debug("Unable to delete collection")
		return []byte("Message: CustomHTTP->StatusMethodNotAllowed\n"), http.StatusMethodNotAllowed
	}

	// If the resource can never be deleted return Status not allowed (405)
	if !r.resource.Deletable {
		log.Debug("Resource is not deletable")
		return []byte("Message: CustomHTTP->StatusMethodNotAllowed\n"), http.StatusMethodNotAllowed
	}

	// Return 404 or success code if the resource has already been deleted
	// This is handled by the resource not existing when the Http delete operation is performed

	// Retrieve JSON repsentation of resource before deletation
	responsePayload, err := r2s.BuildStruct(r.resource.Properties, r.uri)
	if err != nil {
		log.WithField("request", r).Error("Unable to build struct")
	}
	jsonPayload, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, http.StatusInternalServerError
	}

	// Run any before hooks
	event := hooks.Event{
		HTTPOperation: r.httpOperation,
		URI:           r.uri,
		Resource:      r.resource,
		Property:      r.property,
	}
	ctx := map[string]interface{}{}
	hookResponse, hookStatus := rs.runBeforeHookIns(http.MethodDelete, event, ctx, nil)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	// Resource deletion
	// - Remove all of the keys from redis associated with this resource, if they exist
	// - Remove the resource from its containing collection
	rs.rfd.RedisRemoveResource(r.resource, r.uri)

	tokens := strings.Split(r.uri, "/")
	collectionTokens := tokens[:len(tokens)-1] // Remove last token
	collectionURI := strings.Join(collectionTokens, "/")

	indexesInUse, err := rs.redis().LRange(collectionURI+"/"+"Members", 0, -1).Result()
	if err != nil {
		panic(err)
	}
	indexToRemove := ""
	for _, index := range indexesInUse {
		linkKey := collectionURI + "/" + "Members" + "/" + index + "/" + "@odata.id"
		link, _ := rs.redis().Get(linkKey).Result()

		if link == r.uri {
			indexToRemove = index
			break
		}
	}
	if indexToRemove == "" {
		panic("Link Index not found for: " + r.uri)
	}

	// Remove array entry for this link
	log.Trace("LREM ", collectionURI+"/"+"Members", 0, indexToRemove)
	rs.redis().LRem(collectionURI+"/"+"Members", 0, indexToRemove)

	// Remove link related keys
	log.Trace("DEL ", collectionURI+"/"+"Members"+"/"+indexToRemove)
	rs.redis().Del(collectionURI + "/" + "Members" + "/" + indexToRemove)

	log.Trace("DEL ", collectionURI+"/"+"Members"+"/"+indexToRemove+"/"+"@odata.id")
	rs.redis().Del(collectionURI + "/" + "Members" + "/" + indexToRemove + "/" + "@odata.id")

	// Run any after hooks
	hookResponse, hookStatus = rs.runAfterHookIns(http.MethodDelete, event, ctx, responsePayload)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	// Success
	// 200 Status code
	// Return JSON representation of the resource before deletion
	return jsonPayload, http.StatusOK
}

/* handlePatchRequest performs updates on resources on a property level.
 * TODO: Current does not support JSON arrays in payload. */
func (rs *redfishServer) handlePatchRequest(r httpRequest, r2s *dispatcher.Redis2Interface) ([]byte, int) {
	if r.resource == nil {
		return rs.resourceMissing(r.uri)
	}

	// Check Privilege
	// privileges := rs.privilegeRegistry.RequiredPrivileges(r.httpOperation, "", r.resource.Name, "")
	// if !r.account.OperationAllowed(privileges) {
	// 	log.Warn(r.account.UserName + " does not have premission for GET on " + r.uri)
	// 	msg, _ := rs.rfd.GetRedfishMessageString("Base", "InsufficientPrivilege")
	// 	return []byte(msg), http.StatusUnauthorized
	// }

	log.Debug("Http request body")
	log.Debug(string(r.requestBody))
	var postBodyMap map[string]interface{}
	if err := json.Unmarshal(r.requestBody, &postBodyMap); err != nil {
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "MalformedJSON")
		return []byte(msg), http.StatusBadRequest
	}
	// Note: Empty JSON payloads are allowed

	// Note: Privilege checking needs to be done on a per property basis, which is done in the callback

	// Patching a resource
	// - If the resource or all properties can never be updated, HTTP status code 405 shall be returned.
	// - If the client specifies a PATCH request against a Resource Collection, HTTP status code 405
	// should be returned.
	// - In the case of a request including modification to several properties, if one or more properties in
	// the request can never be updated, such as when a property is read only, unknown, or
	// unsupported, an HTTP status code of 200 shall be returned along with a representation of the
	// Redfish Scalable Platforms Management API Specification DSP0266
	// 38 Published Version 1.6.1
	// resource containing a Message annotation specifying the non-updatable properties. In this
	// success case, other properties may be updated in the resource.
	// - In the case of a request modifying a single property, if the property in the request can never be
	// updated, such as when the property is read only, unknown, or unsupported, an HTTP status
	// code of 400 shall be returned along with a representation of the resource containing a Message
	// annotation specifying the non-updatable property.
	// - The PATCH operation should be idempotent in the absence of outside changes to the resource,
	// though the original ETag value may no longer match.
	// - Services may accept a PATCH with an empty JSON object. An empty JSON object in this
	// context means no changes to the resource are being requested.

	// JSON arrays
	// Services may have null entries for properties that are JSON arrays to show the number of entries a client
	// is allowed to use in a PATCH request. Within a PATCH request, unchanged members within a JSON
	// array may be specified as empty JSON objects, and clearing members within a JSON array may be
	// specified with null.

	// Odata annotations
	// OData annotations shall be ignored by the service on updates. This includes things matching the forms of
	//  - PropertyName@odata.term
	//  - @odata.TermName
	// If an Update request only contains OData annotations, the service
	// should return the NoOperation message defined in the Base Message Registry. In order to gain the
	// protection semantics of an etag, the If-Match or If-None-Match header shall be used for that protection
	// and not the @odata.etag property value.

	// Patch can not update collection
	if r.resource.IsCollection {
		log.Debug("Unable to update collection")
		return []byte("Message: CustomHTTP->StatusMethodNotAllowed\n"), http.StatusMethodNotAllowed
	}

	// Is this resource updatable?
	if !r.resource.Updatable {
		log.Debug("Resource is unupdatable")
		return []byte("Message: CustomHTTP->StatusMethodNotAllowed\n"), http.StatusMethodNotAllowed
	}

	// Run any before hooks
	event := hooks.Event{
		HTTPOperation: r.httpOperation,
		URI:           r.uri,
		Resource:      r.resource,
		Property:      r.property,
	}
	ctx := map[string]interface{}{}
	hookResponse, hookStatus := rs.runBeforeHookIns(http.MethodPatch, event, ctx, postBodyMap)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	// Find all properties that are going to be patched
	// TODO: The following are not being logged for messages in the response payload. Bad types for fields, bad value
	// formats, values not in enumerations
	readonlyFields := [][]string{}              // The property is marked as read only
	unknownFields := [][]string{}               // The property is not defined in the schema
	insufficientPrivilegeFields := [][]string{} // Do not have sufficient privilege to set this field

	// Number of fields in request body payload
	fieldCount := 0
	cb := func(property *rfschema.Property, path []string, value interface{}, unknown bool) bool {
		fieldName := path[len(path)-1]

		// Ignore annotations
		odataMatcher := regexp.MustCompile("^([a-zA-Z_][a-zA-Z0-9_]*)@(odata|Redfish|Message)\\.[a-zA-Z_][a-zA-Z0-9_.]+$")
		if odataMatcher.MatchString(fieldName) {
			return false
		}

		fieldCount++
		if unknown {
			unknownFields = append(unknownFields, path)
			return false
		}

		if property.ReadOnly {
			readonlyFields = append(readonlyFields, path)
			return false
		}

		// Check Privilege
		// The privilege map only contains the first "layer" of properties
		if len(path) == 1 {
			privileges := rs.privilegeRegistry.RequiredPrivileges(r.httpOperation, "", r.resource.Name,
				fieldName)
			if !r.account.OperationAllowed(privileges) {
				log.WithFields(log.Fields{
					"username":      r.account.UserName,
					"uri":           r.uri,
					"httpOperation": r.httpOperation,
				}).Warning("User does not have required privileges")
				insufficientPrivilegeFields = append(insufficientPrivilegeFields, path)
				return false
			}
		}

		return true
	}
	redisData, err := rs.rfd.Interface2Redis([]string{}, r.resource.Properties, postBodyMap, cb)
	if err != nil {
		panic(err)
	}

	err = rs.rfd.RedisPatch(redisData, r.uri)
	if err != nil {
		panic(err)
	}

	// Get current representation of resource
	responsePayload, err := r2s.BuildStruct(r.resource.Properties, r.uri)
	if err != nil {
		log.WithField("request", r).Error("Unable to build struct")
	}

	// Add message annotations for un-updatable fields
	for _, path := range readonlyFields {
		fieldName := path[len(path)-1]
		rs.rfd.AddPropertyMessage(responsePayload, path, "Base", "PropertyNotWritable", fieldName)
	}

	for _, path := range unknownFields {
		fieldName := path[len(path)-1]
		rs.rfd.AddPropertyMessage(responsePayload, path, "Base", "PropertyUnknown", fieldName)
	}

	for _, path := range insufficientPrivilegeFields {
		fieldName := path[len(path)-1]
		rs.rfd.AddPropertyMessage(responsePayload, path, "Base", "InsufficientPrivilege", fieldName)
	}

	jsonPayload, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, http.StatusInternalServerError
	}

	status := http.StatusOK

	// If only 1 field was requested to be modified, and it failed the status should be 400
	if fieldCount == 1 && len(readonlyFields) > 0 || len(unknownFields) > 0 {
		status = http.StatusBadRequest
	}

	// Run any after hooks
	hookResponse, hookStatus = rs.runAfterHookIns(http.MethodPatch, event, ctx, responsePayload)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	return jsonPayload, status
}

/* handlePutRequest completely replaces an already existing resource */
func (rs *redfishServer) handlePutRequest(r httpRequest, r2s *dispatcher.Redis2Interface) ([]byte, int) {
	if r.resource == nil {
		return rs.resourceMissing(r.uri)
	}

	// Check Privilege
	privileges := rs.privilegeRegistry.RequiredPrivileges(r.httpOperation, "", r.resource.Name, "")
	if !r.account.OperationAllowed(privileges) {
		log.WithFields(log.Fields{
			"username":      r.account.UserName,
			"uri":           r.uri,
			"httpOperation": r.httpOperation,
		}).Warning("User does not have required privileges")
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "InsufficientPrivilege")
		return []byte(msg), http.StatusUnauthorized
	}

	if r.resource.IsCollection {
		return []byte("Message: CustomHTTP->ResourceIsCollection"), http.StatusMethodNotAllowed
	}

	log.Debug("Http request body")
	log.Debug(string(r.requestBody))

	var postBodyMap map[string]interface{}
	if err := json.Unmarshal(r.requestBody, &postBodyMap); err != nil {
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "MalformedJSON")
		return []byte(msg), http.StatusBadRequest
	}

	if len(postBodyMap) == 0 {
		msg, _ := rs.rfd.GetRedfishMessageString("Base", "EmptyJSON")
		return []byte(msg), http.StatusBadRequest
	}

	// Validate request body
	report := checkResource(r.resource, postBodyMap)
	printObjectResport(report)

	// Make sure all required properties are present
	if len(report.MissingRequired) > 0 {
		log.Debug("Missing required property ", strings.Join(report.getMissingRequired(), ", "))

		// Return an array of redfish messages, and the message for each missing property
		messages := []interface{}{}
		for missing := range report.MissingRequired {
			msg, err := rs.rfd.GetRedfishMessage("Base", "PropertyMissing", missing)
			if err != nil {
				panic(err)
			}
			messages = append(messages, msg)
		}

		payload, err := json.Marshal(messages)
		if err != nil {
			panic(err)
		}
		return payload, http.StatusBadRequest
	}

	// Make sure all required on create properties are present
	if len(report.MissingRequiredOnCreate) > 0 {
		log.Debug("Missing required on create property", strings.Join(report.getMissingRequiredOnCreate(), ", "))
		// Return an array of redfish messages, and the message for each missing property
		messages := []interface{}{}
		for missing := range report.MissingRequired {
			msg, err := rs.rfd.GetRedfishMessage("Base", "CreateFailedMissingReqProperties", missing)
			if err != nil {
				panic(err)
			}
			messages = append(messages, msg)
		}

		payload, err := json.Marshal(messages)
		if err != nil {
			panic(err)
		}
		return payload, http.StatusBadRequest
	}

	// Run any before hooks
	event := hooks.Event{
		HTTPOperation: r.httpOperation,
		URI:           r.uri,
		Resource:      r.resource,
		Property:      r.property,
	}
	ctx := map[string]interface{}{}
	hookResponse, hookStatus := rs.runBeforeHookIns(http.MethodPut, event, ctx, postBodyMap)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	// Delete the all of the keys of the existing resource
	// TODO: Should we perserve things like links, from the pervious resource?
	rs.rfd.RedisRemoveResource(r.resource, r.uri)

	// Add new keys for resource
	data, err := rs.rfd.Interface2Redis([]string{}, r.resource.Properties, postBodyMap, nil)
	if err != nil {
		panic(err)
	}

	err = rs.rfd.RedisInsert(data, r.uri)
	if err != nil {
		panic(err)
	}

	// JSON representation of the newly created resource after the request has been applied.
	responsePayload, err := r2s.BuildStruct(r.resource.Properties, r.uri)
	if err != nil {
		log.WithField("request", r).Error("Unable to build struct")
	}
	jsonPayload, err := json.Marshal(responsePayload)
	if err != nil {
		return nil, http.StatusInternalServerError
	}

	// DEBUG: Pretty print from server
	printStructAsJSON(responsePayload)

	// Run any after hooks
	hookResponse, hookStatus = rs.runAfterHookIns(http.MethodPut, event, ctx, responsePayload)
	if hookResponse != nil {
		return hookResponse, hookStatus
	}

	return jsonPayload, http.StatusOK
}
