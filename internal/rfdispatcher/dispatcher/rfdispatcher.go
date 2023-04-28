// MIT License
//
// (C) Copyright [2018-2023] Hewlett Packard Enterprise Development LP
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
 * A Redfish dispatch server based on DMTF schema files
 *
 */
package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/backend_helpers"
	_ "github.com/Cray-HPE/hms-redfish-translation-service/internal/logger"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/pdu_credential_store"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfschema"
	"google.golang.org/api/compute/v1"

	"github.com/go-redis/redis"
	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

// Error indicating the file handler couldn't perform any action, which does NOT mean it failed during an action.
var ErrFileHandlerNoAction = errors.New("file handler unable to perform any action")

type DispatchHandlerOp int

const (
	InputOp DispatchHandlerOp = iota
	OutputOp
	ActionOp
)

type RedfishDispatcher struct {
	// Server properties
	scriptDirPrefix string
	programName     string

	// Schema tree members
	schemaTree   *rfschema.Tree
	SchemaConfig *rfschema.Config

	redis        *redis.Client
	RedisNetwork string
	RedisAddr    string

	messageRegistries map[string]interface{}

	additionalCollectionCBs []rfschema.CollectionCB

	// Interface for calling backend helpers.
	BackendHelpers []backend_helpers.BackendHelper

	// Services that device backend helpers might need to hook into
	CertificateService *backend_helpers.CertificateService

	ctx    context.Context
	cancel context.CancelFunc
}

func (rfd *RedfishDispatcher) Redis() *redis.Client {
	return rfd.redis
}

func (rfd *RedfishDispatcher) SchemaTree() *rfschema.Tree {
	return rfd.schemaTree
}

func NewDispatcher(ctx context.Context) *RedfishDispatcher {
	rfd := RedfishDispatcher{}
	var err error
	var ok bool

	rfd.ctx = ctx

	rfd.scriptDirPrefix, ok = os.LookupEnv("SCRIPT_DIR_PREFIX")
	if !ok {
		rfd.scriptDirPrefix = "/"
	}
	log.WithField("scriptDirPrefix", rfd.scriptDirPrefix).Info("Script dir prefix")

	rfd.programName = path.Base(os.Args[0])

	// Configuration from environment variables
	rfd.SchemaConfig, err = rfschema.SetupEnvConfig()
	if err != nil {
		panic(err)
	}

	redisHostname, okHostname := os.LookupEnv("REDIS_HOSTNAME")
	redisPort, okPort := os.LookupEnv("REDIS_PORT")
	if !okHostname || !okPort {
		panic("Redis hostname/port not set")
	}
	rfd.RedisAddr = redisHostname + ":" + redisPort

	rfd.RedisNetwork, ok = os.LookupEnv("REDIS_NETWORK")
	if !ok {
		rfd.RedisNetwork = "tcp"
	}

	rfd.schemaTree, err = rfschema.LoadTreeFromConfig(rfd.SchemaConfig)
	if err != nil {
		log.WithFields(log.Fields{
			"err":          err,
			"schemaConfig": rfd.SchemaConfig,
		}).Error("Failed to load schema tree")
		panic(err)
	}

	log.WithFields(log.Fields{
		"RedisNetwork": rfd.RedisNetwork,
		"RedisAddr":    rfd.RedisAddr,
	}).Info("Connecting to redis")
	rfd.redis = redis.NewClient(&redis.Options{
		Network: rfd.RedisNetwork,
		Addr:    rfd.RedisAddr,
	})

	pingResult, err := rfd.redis.Ping().Result()
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Error("Failed to Ping REDIS")
		panic(err)
	}

	if pingResult == "PONG" {
		log.WithField("rfd.RedisAddr", rfd.RedisAddr).Info("Connected to Redis.")
	} else {
		panic("PING'd Redis, but it didn't PONG back")
	}

	rfd.InitMessageRegistries()

	// Core backend helpers
	rfd.CertificateService = &backend_helpers.CertificateService{
		RedisHelper: backend_helpers.RedisHelper{
			Redis: rfd.redis,
		},
	}

	rfd.BackendHelpers = []backend_helpers.BackendHelper{
		rfd.CertificateService,
	}

	// Select the device Specifc backend helper
	backendHelperName, backendHelperSet := os.LookupEnv("BACKEND_HELPER")
	if backendHelperSet {
		var deviceBackendHelper backend_helpers.BackendHelper
		switch backendHelperName {
		case "JAWS":
			deviceBackendHelper = &backend_helpers.JAWSBackedHelper{
				CertificateService: rfd.CertificateService,
				RedisHelper: backend_helpers.RedisHelper{
					Redis: rfd.redis,
				},
				KnownDevices: make(map[string]pdu_credential_store.Device),
			}
		case "GCloud":
			deviceBackendHelper = &backend_helpers.GCloudHelper{
				CertificateService: rfd.CertificateService,
				RedisHelper: backend_helpers.RedisHelper{
					Redis: rfd.redis,
				},
				KnownInstances: make(map[string]*compute.Instance),
			}
		case "RFSimulator":
			deviceBackendHelper = &backend_helpers.RFSimulatorHelper{
				CertificateService: rfd.CertificateService,
				RedisHelper: backend_helpers.RedisHelper{
					Redis: rfd.redis,
				},
				KnownInstances: make(map[string]interface{}),
			}
		case "Mock":
			deviceBackendHelper = &backend_helpers.MockBackendHelper{
				CertificateService: rfd.CertificateService,
				RedisHelper: backend_helpers.RedisHelper{
					Redis: rfd.redis,
				},
			}
		case "SNMPSwitch":
			deviceBackendHelper = &backend_helpers.SNMPSwitchHelper{
				CertificateService: rfd.CertificateService,
				RedisHelper: backend_helpers.RedisHelper{
					Redis: rfd.redis,
				},
				KnownDevices: make(map[string]backend_helpers.SNMPDevice),
			}
		default:
			log.WithField("backendHelperName", backendHelperName).Panic("Unknown backend helper provided via BACKEND_HELPER env variable")
		}

		rfd.BackendHelpers = append(rfd.BackendHelpers, deviceBackendHelper)
	}

	for _, backendHelpler := range rfd.BackendHelpers {
		err = backendHelpler.SetupBackendHelper(ctx, nil)
		if err != nil {
			log.WithField("err", err).Panic("Backend helper setup function did not complete without error!")
		}

	}

	return &rfd
}

func (rfd *RedfishDispatcher) RunPeriodic() {
	var err error
	env := make(map[string]interface{})

	for _, backendHelper := range rfd.BackendHelpers {
		err = backendHelper.RunPeriodic(rfd.ctx, env)
		if err != nil {
			// TODO log which one failed?
			log.WithField("err", err).Error("Backend helper setup function did not complete without error!")
		}
	}

}

func (rfd *RedfishDispatcher) RegisterCollectionCB(cb rfschema.CollectionCB) {
	rfd.additionalCollectionCBs = append(rfd.additionalCollectionCBs, cb)
}

// Returns resource, used for gets. It needs to be able to return properties too for POST actions.
func (rfd *RedfishDispatcher) GetResource(uri string, xname string) (*rfschema.Resource, *rfschema.Property) {
	collectionCB := func(collection *rfschema.Resource, baseCollectionPath string, instance string) bool {
		collectionPath := xname + baseCollectionPath

		log.WithFields(log.Fields{
			"collection.Name": collection.Name,
			"collectionPath":  collectionPath,
			"instance":        instance,
		}).Trace("Collection callback")
		// Check redis for collection member existence
		if _, present := collection.Properties[instance]; present {
			// The name of the instance shares the name with one of the collection's properties, this is not a valid
			// instance name
			log.WithFields(log.Fields{
				"instance":       instance,
				"collectionPath": collectionPath,
			}).Warning("The instance name is invalid for the collection")
			return false
		}

		exist, err := rfd.redis.Exists(collectionPath + "/" + instance).Result()
		if err != nil {
			panic(err)
		}
		if exist > 0 {
			return true
		}

		// Try other collection callbacks registered with the dispatcher
		log.Trace("Running additional callbacks")
		for i, cb := range rfd.additionalCollectionCBs {
			log.Trace("Running additional callback: ", i)
			if cb(collection, collectionPath, instance) {
				return true
			}
		}

		// Not member of the collection
		return false
	}

	return rfschema.WalkTree(rfd.schemaTree, collectionCB, uri)
}

/* Helper function to add extended info messages to properties in an object. */
func (rfd *RedfishDispatcher) AddPropertyMessage(obj map[string]interface{}, path []string, registryPrefix, messageID string, args ...interface{}) error {
	if len(path) < 1 {
		return errors.New("Path must contain at least one element")
	}

	// Get message
	msg, err := rfd.GetRedfishMessage(registryPrefix, messageID, args...)
	if err != nil {
		return err
	}

	// Add message to object
	pathToObj := path[:len(path)-1]
	fieldName := path[len(path)-1]
	current := obj

	for _, token := range pathToObj {
		next, ok := current[token]
		if !ok {
			return errors.New("Path token not present in object: " + token)
		}
		current = next.(map[string]interface{})
	}

	annotation := fieldName + "@Message.ExtendedInfo"
	var msgArray []interface{}
	if _, ok := current[annotation]; ok {
		// An array of messages already exists
		arrayRaw := current[annotation]
		msgArray = arrayRaw.([]interface{})
	}

	msgArray = append(msgArray, msg)
	current[annotation] = msgArray

	return nil
}

// Helper function to read from or execute files for a given URI. There are 5 possibilities for this function:
//   1) Redis contains a key that indicates there is a handler override: this file will be executed with the key
//      given as an argument.
//   2) There exists on the filesystem at a path matching the URI an executable file: this file will be executed.
//   3) There exists on the filesystem at a path matching the URI a regular file: the contents of this file will be
//      returned.
//   4) There exists on the filesystem at a predetermined path a sort of "global" executable file: this file will be
//      executed with the key given as an argument. This file exists to solve the problem of a lot of replication
//      in many other smaller files. If there are a lot of actions that do a lot of the same things this file can be
//      used to unite the majority of that logic.
//   5) None of the above. Return back an error indicating the file handler couldn't do anything.
// TODO: Might be worth while to add the ability to execute Go code instead of just Bash scripts.
func (rfd *RedfishDispatcher) DoFileHandler(op DispatchHandlerOp, uri string, args []string,
	env map[string]string) ([]byte, error) {
	// 1 - Check to see if there is a handler override in Redis.
	handlerKey := uri
	if op == ActionOp {
		// As I understand it this whole Target thing comes about by the fact that the "Target" field as it's
		// presented in the JSON body isn't actually a field, it's just added to the end.
		// An example of this would be be a post to /Actions/#Chassis.Reset...
		// as this is an "action" operation so the get needs to be on Actions/#Chassis.Reset/Target.
		handlerKey += "/Target"
	}
	handlerKey += ":handler"

	handlerExists, err := rfd.redis.Exists(handlerKey).Result()
	if err != nil {
		return []byte(""), err
	}
	if handlerExists > 0 {
		handlerPath, err := rfd.redis.Get(handlerKey).Result()
		if err != nil {
			return []byte(""), err
		}

		logFields := log.Fields{
			"handlerPath": handlerPath,
			"uri":         uri,
		}

		fi, err := os.Stat(handlerPath)
		if err != nil {
			detailedError := errors.New("handler path in Redis does not exist")
			log.WithFields(logFields).Error(detailedError.Error())
			return []byte(""), detailedError
		}

		// Check to see if the file has the executable bit set.
		if (fi.Mode().Perm() & 0111) <= 0 {
			detailedError := errors.New("handler path in Redis can not be executed")
			log.WithFields(logFields).Error(detailedError.Error())
			return []byte(""), detailedError
		}

		log.WithFields(logFields).Debug("Found handler path in Redis, executing as file")

		// Pass uri/key to the executable for some context.
		return executeFile(handlerPath, append(args, uri), env)
	}

	// 2 - Check to see if a file matching exactly the URI exists on the filesystem.
	var file string
	if op == ActionOp {
		file = filepath.Join(rfd.scriptDirPrefix, uri, "Target")
	} else {
		file = filepath.Join(rfd.scriptDirPrefix, uri)
	}
	fi, err := os.Stat(file)
	if err == nil {
		if (fi.Mode().Perm() & 0111) > 0 {
			return executeFile(file, args, env)
		} else {
			// 3 - Regular file, get the contents.
			switch op {
			case OutputOp:
				contents, err := ioutil.ReadFile(file)
				if err != nil {
					detailedError := errors.New("Failed to read file")
					log.WithFields(log.Fields{
						"file": file,
						"err":  err,
					}).Error(detailedError.Error())
					return []byte(""), detailedError
				}
				return contents, nil
			case InputOp:
				// TODO Add write support
			case ActionOp:
				// TODO How would you perform an actions with read/write on a file?
			default:
				return []byte(""), errors.New("Unsupported file handler operation")
			}
		}
	}

	// 4 - Check to see if there is a more universal file we can use in the PATH.
	rtlExecutor, err := exec.LookPath("redfish-translation-layer-executor")
	if err == nil {
		return executeFile(rtlExecutor, append(args, uri), env)
	}

	// 5 - Not our lucky day.
	return []byte(""), ErrFileHandlerNoAction
}

// Execute the given executable at the given path with provided arguments and environment variables.
// Return the program's standard output.
func executeFile(script string, args []string, env map[string]string) ([]byte, error) {
	cmd := exec.Command(script, args...)

	// Append all of the provided environment variables.
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdout, err := cmd.Output()
	if err != nil {
		log.Error(err)
		return nil, errors.New("Script execution failed: " + script)
	}
	return []byte(stdout), nil
}

/* Helper function to perform a IPC action with the backend */
func (rfd *RedfishDispatcher) PerformIPC(targetkey string, request map[string]interface{}) (map[string]string, error) {
	// Generate Id
	// TODO This could also be replaced with a counter from redis
	id := uuid.New().String()
	log.Debug("IPC ID: ", id)

	keyspace := "Cray:" + rfd.programName + ":IPC:" + id
	requestKey := keyspace + ":Request"
	responseKey := keyspace + ":Response"

	// Set response key location for backend
	request["ResponseKey"] = responseKey

	// Insert request hash into redis
	err := rfd.redis.HMSet(requestKey, request).Err()
	if err != nil {
		return nil, err
	}

	// Subscribe to the message done key before attempting to singal the backend
	pubsub := rfd.redis.Subscribe("__keyspace@0__:" + responseKey)
	sub := pubsub.Channel()
	defer pubsub.Close()

	// Rename the request key to target key it does not exist. This is an atomic operation that will move
	// the hash from its initial key to the location of the target key
	// Reply:
	// 1 - Key was renamed, the backend had been signaled with our request
	// 0 - Key was not set, it already exists. The backend is currently performing an operation.
	//     Either block and wait for the keys deletion, or return error.
	keyWasSet, err := rfd.redis.RenameNX(requestKey, targetkey).Result()
	if err != nil {
		panic(err)
	}

	// Return an error if the backend is busy
	// TODO: Implement blocking
	if !keyWasSet {
		log.Debug("IPC Operation in progress: ", targetkey)
		return nil, errors.New("Backend is currently performing an action")
	}

	// We now have ownership of the key. It is the responsibility of the backend to delete the key.
	// This allows the backend to control concurrency.

	// Clean up Response/Request keys when the function returns
	defer func() {
		log.Debug("Deleting: ", requestKey, ", ", responseKey)
		count, err := rfd.redis.Del(requestKey, responseKey).Result()
		if err != nil {
			panic(err)
		}
		log.Trace("Deleted ", count, " keys")
	}()

	// Wait for the backend to insert the Response key
	// Timeout IPC after 15 second
	timeout := time.After(15 * time.Second)

	// Since there is no default case select will block until something is placed in either the sub
	// or timeout channels
	select {
	case msg := <-sub:
		log.Debug("Received message for key: ", targetkey)
		log.Debug("Channel: ", msg.Channel)
		log.Debug("Pattern: ", msg.Pattern)
		log.Debug("Payload: ", msg.Payload)
	case <-timeout:
		log.Warn("IPC operation timed out")
		return nil, errors.New("IPC operation timed out")
	}

	// Get response
	response, err := rfd.redis.HGetAll(responseKey).Result()
	if err != nil {
		return nil, err
	}

	// Done
	return response, nil
}

// HandleAction is a helper function for taking care of actions.
func (rfd *RedfishDispatcher) HandleAction(property *rfschema.Property, uri string, body []byte,
	host string) (responseBody []byte, err error) {
	logFields := log.Fields{
		"key":  uri,
		"host": host,
		"body": string(body),
	}

	var postBodyMap map[string]interface{}
	var args []string
	_, err = rfd.redis.SMembers(uri).Result()
	if err != nil {
		log.WithFields(logFields).Error("Unable to find URI in Redis, action not supported")
		return
	}
	if err = json.Unmarshal(body, &postBodyMap); err != nil {
		log.WithFields(logFields).Error("Malformed JSON")
		return
	}
	if len(postBodyMap) == 0 {
		log.WithFields(logFields).Error("Body JSON empty")
		return
	}

	// TODO: Check the AllowableValues against what has been passed.
	// Check to make sure this is a legal action.
	//x0p1/redfish/v1/PowerEquipment/RackPDUs/A/Outlets/AA3/Actions
	//#Outlet.PowerControl/PowerControl@Redfish.AllowableValues
	//lastSlashIndex := strings.LastIndex(uri, "/")
	//keyspace := uri[:lastSlashIndex]
	//action := "#" + uri[lastSlashIndex + 1:]
	//
	//keyspaceMembers, err := rfd.redis.SMembers(keyspace).Result()
	//found := false
	//for _, member := range keyspaceMembers {
	//	if member == action {
	//		found = true
	//		break
	//	}
	//}
	//
	//if !found {
	//	log.WithFields(logFields).Error("Action is not supported")
	//	return
	//}

	/*
	 * File handler
	 */
	for key := range postBodyMap {
		value, _ := json.Marshal(postBodyMap[key])
		args = append(args, key, string(value))
	}

	responseBody, err = rfd.DoFileHandler(ActionOp, uri, args, nil)
	if err == nil {
		logFields["responseBody"] = string(responseBody)
		log.WithFields(logFields).Debug("Response from DoFileHandler")
		return
	}

	/*
	 * IPC
	 */
	// Does this key have an associated IPC key?
	targetKey := uri + "/Target"
	ipcKey := targetKey + ":ipc"
	ipcExists, err := rfd.redis.Exists(ipcKey).Result()
	if err != nil {
		logFields["err"] = err
		logFields["ipcKey"] = ipcKey
		log.WithFields(logFields).Panic("Unable to check ipcKey existence")
	}
	if ipcExists > 0 {
		request := map[string]interface{}{}

		// Pass Action parameters to the request
		for name, value := range postBodyMap {
			request[name] = value
		}

		var response map[string]string
		response, err = rfd.PerformIPC(targetKey, request)
		if err != nil {
			log.WithFields(logFields).Error("IPC call failed")
			return
		}

		responseBody, err = json.Marshal(response)
		if err != nil {
			log.WithFields(logFields).Error("Unable to marshal IPC response")
			return
		}
		logFields["responseBody"] = string(responseBody)
		log.WithFields(logFields).Debug("Response from IPC")

		return
	}

	// The other thing we will always do is call a BackendHelper which could either be a real function or just a mock
	// no-op style call.
	for _, backendHelper := range rfd.BackendHelpers {
		if host == "" {
			log.WithFields(logFields).Panic("BackendHelper is set but there is no host set")
		}
		var env map[string]string
		env, err = backendHelper.GetEnvForXname(host)
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Panic("Failed to get ENV for host")
		}

		// Need to include the body in the environment so the action can be known.
		for k, v := range postBodyMap {
			switch value := v.(type) {
			case string:
				env[k] = value
			case map[string]interface{}:
				// Right now the only thing in an Action that we are expected to be in a
				// nested object is an OData ID Link. In the future when we need to
				// actually process an nested object we will need to reevaluate how
				// we build up the environment for backend helpers.
				odataIDRaw, ok := value["@odata.id"]
				if !ok {
					// HACK: Just make it a string, this could be improved.
					env[k] = fmt.Sprintf("%v", v)
					continue
				}

				odataID, ok := odataIDRaw.(string)
				if !ok {
					// HACK: Just make it a string, this could be improved.
					env[k] = fmt.Sprintf("%v", v)
					continue
				}

				env[k] = odataID
			default:
				// HACK: Just make it a string, this could be improved.
				env[k] = fmt.Sprintf("%v", v)
			}
		}

		// Define a context with a timeout.
		timeout := 30 * time.Second
		if err != nil {
			log.WithField("err", err).Fatal("Unable to parse timeout duration")
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		var backendValue string
		backendValue, err = backendHelper.RunBackendHelper(ctx, uri, nil, env)
		if err == backend_helpers.ErrBackendContinue {
			log.WithFields(logFields).Debug("Backend does not apply, going to next in line")
			continue
		} else if err == nil {
			// Will assume that if we do get something back it's JSON, but we probably didn't.
			if backendValue != "" {
				responseBody, jsonErr := json.Marshal(backendValue)
				if jsonErr != nil {
					log.WithFields(logFields).Error("Unable to marshal backend helper response")
					return nil, jsonErr
				}
				logFields["responseBody"] = string(responseBody)
				log.WithFields(logFields).Debug("Response from backend helper")
			}
			log.Debug("Backend helper returned successful")

			return nil, nil
		} else {
			logFields["backendErr"] = err
			break
		}
	}

	log.WithFields(logFields).Error("Unable to handle action")
	return
}
