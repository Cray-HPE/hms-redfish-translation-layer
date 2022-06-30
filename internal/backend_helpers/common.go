// MIT License
//
// (C) Copyright [2020-2022] Hewlett Packard Enterprise Development LP
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

package backend_helpers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	base "github.com/Cray-HPE/hms-base"
	compcredentials "github.com/Cray-HPE/hms-compcredentials"
	hmshttp "github.com/Cray-HPE/hms-go-http-lib"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/telemetry"
	securestorage "github.com/Cray-HPE/hms-securestorage"
	rf "github.com/Cray-HPE/hms-smd/pkg/redfish"
	"github.com/go-redis/redis"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Generic variables
const ServiceRootKeyspace = "/redfish/v1"
const ChassisKeyspace = ServiceRootKeyspace + "/Chassis"
const SystemsKeyspace = ServiceRootKeyspace + "/Systems"

const GenericBmcID = "BMC"
const GenericCertificateID = "1"

var httpClient *retryablehttp.Client
var compCredStore compcredentials.CompCredStore

var RFEventID uint64 = 1

// Get the service/instance name, create UserAgent headers.

func getSvcInstName() map[string]string {
	sn, err := base.GetServiceInstanceName()
	if err != nil {
		sn = "RTS_BE"
	}
	hdr := make(map[string]string)
	hdr[base.USERAGENT] = sn
	return hdr
}

// Structure for making Redis stuff generic
type RedisHelper struct {
	Redis               *redis.Client
	RedisActivePipeline redis.Pipeliner
}

func (helper RedisHelper) startPipeline() redis.Pipeliner {
	return helper.Redis.Pipeline()
}

func (helper RedisHelper) initAccount(username string, role string, password string) (err error) {
	logFields := log.Fields{
		"username": username,
		"role":     role,
	}

	toHash := []byte(password)
	hash, err := bcrypt.GenerateFromPassword(toHash, bcrypt.DefaultCost)
	if err != nil {
		return
	}

	passwordHash := hex.EncodeToString(hash)

	instanceName := username
	enabled := "true"
	locked := "false"
	roleId := role

	accountServiceKeyspace := filepath.Join(ServiceRootKeyspace, "AccountService")
	accountsKeyspace := filepath.Join(accountServiceKeyspace, "Accounts")

	// Setup role
	instanceKeyspace := filepath.Join(accountsKeyspace, instanceName)

	helper.addMemberToSet(instanceKeyspace, "@odata.id")
	helper.addMemberToSet(instanceKeyspace, "@odata.type")
	helper.addMemberToSet(instanceKeyspace, "@odata.context")
	helper.setValueForPropertyInKeyspace(instanceKeyspace, "Name", instanceName)
	helper.setValueForPropertyInKeyspace(instanceKeyspace, "Id", instanceName)
	helper.setValueForPropertyInKeyspace(instanceKeyspace, "RoleId", roleId)
	helper.setValueForPropertyInKeyspace(instanceKeyspace, "Enabled", enabled)
	helper.setValueForPropertyInKeyspace(instanceKeyspace, "Locked", locked)
	helper.setValueForPropertyInKeyspace(instanceKeyspace, "UserName", username)

	propertyKeyspace := filepath.Join(instanceKeyspace, "Password:hash")
	err = helper.Redis.Set(propertyKeyspace, passwordHash, 0).Err()

	memberKeyspace, err := helper.addMemberToArray(accountsKeyspace, "Members")
	helper.setValueForPropertyInKeyspace(memberKeyspace, "@odata.id", instanceKeyspace)

	log.WithFields(logFields).Debug("Init account finished")

	return
}

func (helper RedisHelper) initManagerCollection(xname string) (err error) {
	// The service root is special, so we don't want to prepend the xname at the beginning
	// of the key
	keyspace, err := helper.addMemberToSet(ServiceRootKeyspace, "Managers")
	if err != nil {
		return err
	}

	// Here we want to prepend the xname, so this xname gets its own manager collection
	keyspace = filepath.Join(xname, keyspace)

	helper.addMemberToSet(keyspace, "@odata.id")
	helper.addMemberToSet(keyspace, "@odata.type")

	helper.setValueForPropertyInKeyspace(keyspace, "Name", "Managers Collection")
	helper.setValueForPropertyInKeyspace(keyspace, "Description", "A collection of all Manager instances on this host")

	// Initialize empty members array
	helper.addMemberToSet(keyspace, "Members")

	return nil
}

func (helper RedisHelper) initManager(xname, bmcID, model string) (err error) {
	keyspace := filepath.Join(xname, ManagersKeyspace, bmcID)

	helper.addMemberToSet(keyspace, "@odata.id")
	helper.addMemberToSet(keyspace, "@odata.type")
	helper.setValueForPropertyInKeyspace(keyspace, "Id", bmcID)
	helper.setValueForPropertyInKeyspace(keyspace, "Name", "Redfish Translation Service")
	helper.setValueForPropertyInKeyspace(keyspace, "Model", model)
	helper.setValueForPropertyInKeyspace(keyspace, "PowerState", "On")

	// Add manager to collection
	memberKeyspace, err := helper.addMemberToArray(filepath.Join(xname, ManagersKeyspace), "Members")
	if err != nil {
		return fmt.Errorf("unable to add member to array for instance: %s", xname)
	}

	helper.setValueForPropertyInKeyspace(memberKeyspace, "@odata.id", keyspace)

	return nil
}

// Added a member to a set, which can either be defined by using the setValueForPropertyInKeyspace method or if not
// this dispatcher will be invoked to retrieve/calculate it.
func (helper RedisHelper) addMemberToSet(keyspace string, member string) (memberKeyspace string, err error) {
	logFields := log.Fields{
		"keyspace": keyspace,
		"member":   member,
	}

	if helper.RedisActivePipeline != nil {
		err = helper.RedisActivePipeline.SAdd(keyspace, member).Err()
	} else {
		err = helper.Redis.SAdd(keyspace, member).Err()
	}

	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to add member to set")
	} else {
		memberKeyspace = keyspace + "/" + member
		log.WithFields(logFields).Debug("Added member to set")
	}

	return
}

// Sets the value permanently at a given keyspace for a property thereby making Redis the owner of that information.
// The first step is to set the value of the property, the second is to add that property to the array that should
// contain that value.
func (helper RedisHelper) setValueForPropertyInKeyspace(keyspace string, property string,
	value string) (propertyKeyspace string, err error) {
	logFields := log.Fields{
		"keyspace": keyspace,
		"property": property,
		"value":    value,
	}

	propertyKeyspace = keyspace + "/" + property

	if helper.RedisActivePipeline != nil {
		err = helper.RedisActivePipeline.Set(propertyKeyspace, value, 0).Err()
	} else {
		err = helper.Redis.Set(propertyKeyspace, value, 0).Err()
	}

	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to set value for property")
	} else {
		_, err = helper.addMemberToSet(keyspace, property)
		if err != nil {
			log.WithFields(logFields).Debug("Set value for property in keyspace")
		}
	}

	return
}

func (helper RedisHelper) addMemberToArray(keyspace string, property string) (arrayKeyspace string, err error) {
	logFields := log.Fields{
		"keyspace": keyspace,
		"property": property,
	}

	propertyKeyspace := keyspace + "/" + property
	count, err := helper.Redis.LLen(propertyKeyspace).Result()
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to get length of property keyspace")
		return
	}
	if count == 0 {
		_, err = helper.addMemberToSet(keyspace, property)
		if err != nil {
			return
		}
	}

	err = helper.Redis.RPush(propertyKeyspace, count).Err()
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to add index to array")
	} else {
		arrayKeyspace = propertyKeyspace + "/" + strconv.FormatInt(count, 10)
		logFields["count"] = count
		log.WithFields(logFields).Debug("Added member to array")
	}

	return
}

func (helper RedisHelper) removeXnamePrependedKeys(xname string) (err error) {
	logFields := log.Fields{
		"xname": xname,
	}

	xnameKeys, err := helper.Redis.Keys(xname + "*").Result()
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to get keys for xname")
		return
	}

	pipe := helper.Redis.Pipeline()

	for _, xnameKey := range xnameKeys {
		err := pipe.Del(xnameKey).Err()
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Error("Unable to delete xname key")
		} else {
			log.WithField("key", xnameKey).Debug("Removed key")
		}
	}

	_, err = pipe.Exec()

	return
}

// This function is used to remove keys that should no longer be in Redis. One example would be changing the power
// state on an outlet, it wouldn't make sense to let the cached version remain in the database as it's now different.
func (helper RedisHelper) invalidateRedisKeys(keys []string) {
	logFields := log.Fields{
		"keys": keys,
	}

	for _, key := range keys {
		err := helper.Redis.Del(key).Err()
		if err == nil {
			log.WithFields(logFields).Debug("Removed key")
		} else {
			logFields["err"] = err
			log.WithFields(logFields).Error("Unable to remove key")
		}
	}
}

// Determine the member nanmes/ids that belong in this collection
// The memberKey should point to the key that represents the Member field in a collection
// Such as: xname/redfish/v1/PowerEquipment/RackPDUs/Members
func (helper RedisHelper) collectionMembers(memberKey string) ([]string, error) {
	indexes, err := helper.Redis.LRange(memberKey, 0, -1).Result()
	if err != nil {
		return []string{}, err
	}

	members := []string{}
	for _, index := range indexes {
		key := path.Join(memberKey, index, "@odata.id")
		link, err := helper.Redis.Get(key).Result()
		if err != nil {
			return []string{}, err
		}

		members = append(members, path.Base(link))
	}

	return members, nil
}

// A timeout key can be used to implement a rudimentary timer using redis.
// The timeout key is a just a key with a dummy value that expires, after a set timeout. When the key
// is expired this function will populate redis with a new key that will expire in the defined timeout.
func (helper RedisHelper) checkTimeoutKey(key string, timeout time.Duration) (bool, error) {
	exists, err := helper.Redis.Exists(key).Result()
	if err != nil {
		return false, err
	}

	if exists == 0 {
		// The Key does not exist
		if err := helper.Redis.Set(key, "rts", timeout).Err(); err != nil {
			return false, err
		}

		return true, nil
	}

	// The Key Exists
	return false, nil
}

func informHSM(ctx context.Context, xname string) (err error) {
	return informHSMWithFQDN(ctx, xname, xname+":8083")
}

func informHSMWithFQDN(ctx context.Context, xname, fqdn string) error {
	// We'll use the existence of the HSM_URL environment variable to determine whether we should inform.
	hsmURL, exists := os.LookupEnv("HSM_URL")
	if !exists || hsmURL == "" {
		log.Warning("Unable to inform HSM because HSM_URL isn't set.")
		return nil
	}

	log.WithField("xname", xname).Info("Checking to see if RedfishEndpoint exists in HSM")
	rfEndpoint, _, getErr := getRedfishEndpointFromHSM(ctx, xname)
	if getErr != nil {
		return getErr
	}

	if rfEndpoint == nil {
		log.WithField("xname", xname).Info("RedfishEndpoint does not exist in HSM, will attempt to create one")

		// Build up the Redfish Endpoint structure.
		rediscoverOnUpdate := true
		rfEndpoint := rf.RawRedfishEP{
			ID:             xname,
			FQDN:           fqdn,
			RediscOnUpdate: &rediscoverOnUpdate,
		}

		// Create the Redfish Endpoint in HSM
		_, createErr := createRedfishEndpointInHSM(ctx, rfEndpoint)
		if createErr != nil {
			log.WithField("xname", xname).Error(createErr)
			return createErr
		}

		// Endpoint was created successfully
		log.WithField("xname", xname).Info("RedfishEndpoint created successfully in HSM")

		return nil
	}

	// If the RedfishEndpoint is already present there is nothing to do, as the redfish endpoint currently exists in HSM.
	// TODO in the future think about patching HSM if any of the fields have changed.
	log.WithField("xname", xname).Info("RedfishEndpoint currently exists in HSM")
	return nil
}

func getRedfishEndpointFromHSM(ctx context.Context, xname string) (rfEndpointPtr *rf.RawRedfishEP, statusCode int, err error) {
	// We'll use the existence of the HSM_URL environment variable to determine whether we should get the redfish endpoint.
	hsmURL, exists := os.LookupEnv("HSM_URL")
	if !exists || hsmURL == "" {
		log.Warning("Unable to get RedfishEndpoint in HSM because HSM_URL isn't set.")
		return
	}

	request := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusOK, http.StatusNotFound},
		FullURL:             hsmURL + "/hsm/v2/Inventory/RedfishEndpoints/" + xname,
		Method:              "GET",
		CustomHeaders:       getSvcInstName(),
	}

	responsePayload, statusCode, err := request.DoHTTPAction()
	if err != nil {
		err = fmt.Errorf("attempted to retrieve redfish endpoint from HSM received unexpected status code: %s", err)
		log.WithField("xname", xname).Warning(err.Error())
		return
	}

	if statusCode == http.StatusNotFound {
		// The Endpoint was not found, give back a nil.
		return nil, statusCode, nil
	}

	rfEndpoint := rf.RawRedfishEP{}
	marshalErr := json.Unmarshal(responsePayload, &rfEndpoint)
	if marshalErr != nil {
		marshalErr = fmt.Errorf("attempted to marshal JSON response but failed: %s", marshalErr)
		log.WithField("string(responsePayload)", string(responsePayload)).Error(marshalErr.Error())
		return
	}

	return &rfEndpoint, statusCode, nil
}

func createRedfishEndpointInHSM(ctx context.Context, rfEndpoint rf.RawRedfishEP) (statusCode int, err error) {
	// We'll use the existence of the HSM_URL environment variable to determine whether we should create the redfish endpoint.
	hsmURL, exists := os.LookupEnv("HSM_URL")
	if !exists || hsmURL == "" {
		log.Warning("Unable to create RedfishEndpoint in HSM because HSM_URL isn't set.")
		return
	}

	payload, marshalErr := json.Marshal(rfEndpoint)
	if marshalErr != nil {
		err = fmt.Errorf("attempted to marshal JSON payload but failed: %s", marshalErr)
		log.WithField("rfEndpoint", rfEndpoint).Error(err.Error())
		return
	}

	// Create the RedfishEndpoint as it does not already exist
	informRequest := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusCreated},
		FullURL:             hsmURL + "/hsm/v2/Inventory/RedfishEndpoints",
		Method:              "POST",
		CustomHeaders:       getSvcInstName(),
		Payload:             payload,
	}

	_, statusCode, err = informRequest.DoHTTPAction()
	if err != nil {
		if statusCode == http.StatusConflict {
			log.WithField("xname", rfEndpoint.ID).Info("xname already registered with HSM")
		} else {
			err = fmt.Errorf("attempted to inform HSM about xname but failed: %s", err)
			log.WithField("string(payload)", string(payload)).Error(err.Error())
		}
	} else {
		log.WithField("xname", rfEndpoint.ID).Info("Registered xname with HSM")
	}

	return
}

func deleteFromHSM(ctx context.Context, xname string) (err error) {
	// We'll use the existence of the HSM_URL environment variable
	// to determine whether we can remove from HSM.
	hsmURL, exists := os.LookupEnv("HSM_URL")
	if !exists || hsmURL == "" {
		log.Warning("Unable to delete " +
			xname +
			" from HSM because HSM_URL isn't set.")
		return
	}
	deleteRequest := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusOK},
		FullURL:             hsmURL + "/hsm/v2/Inventory/RedfishEndpoints/" + xname,
		Method:              "DELETE",
		CustomHeaders:       getSvcInstName(),
	}
	_, _, deleteErr := deleteRequest.DoHTTPAction()
	if deleteErr != nil {
		if strings.Contains(deleteErr.Error(), "404") {
			// Nothing to do if it is not there anyway.
			log.WithField("xname", xname).Info("xname not found in HSM")
		} else {
			err = fmt.Errorf("attempted to remove %s from HSM but failed: %s",
				xname, deleteErr)
		}
	} else {
		log.WithField("xname", xname).Info("removed xname from HSM")
	}
	return
}

func informDNS(ctx context.Context, xname string) (err error) {
	logCtx := log.WithField("xname", xname)

	// We'll use the existence of the RTS_DNS_PROVIDER environment variable to determine whether we should create DNS entry.
	dnsProvider, exists := os.LookupEnv("RTS_DNS_PROVIDER")
	if !exists {
		logCtx.Warning("Unable to inform DNS because RTS_DNS_PROVIDER isn't set.")
		return
	}

	switch dnsProvider {
	case "none":
		logCtx.Warning("Not informing DNS as RTS_DNS_PROVIDER is set to 'none'.")
		return
	case "k8s_service":
		created, err := addXNameService("services", xname)
		if err != nil {
			return err
		}

		logCtx.WithField("created", created).Info("Added k8s service for name resoution")
	default:
		log.WithField("RTS_DNS_PROVIDER", dnsProvider).Error("Unknown DNS provider")
		return fmt.Errorf("unknown DNS provider: %s", dnsProvider)
	}
	return
}

func postRFPowerEvent(ctx context.Context, xname string, resource string, powerState string) (err error) {
	// We'll use the existence of the COLLECTOR_URL environment variable to determine whether we should send event.
	collectorURL, exists := os.LookupEnv("COLLECTOR_URL")
	if !exists {
		log.Warning("Unable to POST new RF event because COLLECTOR_URL isn't set.")
		return
	}

	// Build the event.
	record := rf.EventRecord{
		EventId:        strconv.FormatUint(RFEventID, 10),
		EventTimestamp: time.Now().Format(time.RFC3339),
		Severity:       "OK",
		Message: fmt.Sprintf("The power state of resource %s has changed to type %s.",
			resource, powerState),
		MessageId:         "CrayAlerts.1.0.ResourcePowerStateChanged",
		MessageArgs:       []string{resource, powerState},
		OriginOfCondition: rf.ResourceID{Oid: resource},
	}
	event := rf.Event{
		Context:      xname + ":PowerState",
		Events:       []rf.EventRecord{record},
		EventsOCount: 1,
	}

	// Build the payload
	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		err = fmt.Errorf("attemtped to marshal JSON payload but failed: %s", marshalErr)
		log.WithField("event", event).Error(err.Error())
		return
	}

	rfPOST := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusOK},
		FullURL:             collectorURL,
		Method:              "POST",
		CustomHeaders:       getSvcInstName(),
		Payload:             payload,
	}

	RFEventID++

	_, _, informErr := rfPOST.DoHTTPAction()
	if informErr != nil {
		err = fmt.Errorf("attempted to POST RF event but failed: %s", informErr)
		log.WithField("string(payload)", string(payload)).Error(err.Error())
	} else {
		log.WithFields(log.Fields{
			"xname":           xname,
			"resource":        resource,
			"powerState":      powerState,
			"string(payload)": string(payload),
		}).Info("Sent RF event.")
	}

	return
}

func postTelemetryEvent(ctx context.Context, event telemetry.Event) (err error) {
	// We'll use the existence of the COLLECTOR_URL environment variable to determine whether we should send event.
	collectorURL, exists := os.LookupEnv("COLLECTOR_URL")
	if !exists {
		log.Warning("Unable to POST new RF event because COLLECTOR_URL isn't set.")
		return
	}

	// Build the payload
	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		err = fmt.Errorf("attemtped to marshal JSON payload but failed: %s", marshalErr)
		log.WithField("event", event).Error(err.Error())
		return
	}

	rfPOST := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusOK},
		FullURL:             collectorURL,
		Method:              "POST",
		CustomHeaders:       getSvcInstName(),
		Payload:             payload,
	}
	_, _, informErr := rfPOST.DoHTTPAction()
	if informErr != nil {
		err = fmt.Errorf("attempted to POST RF event but failed: %s", informErr)
		log.WithField("string(payload)", string(payload)).Error(err.Error())
	} else {
		log.WithFields(log.Fields{
			// "xname":           xname,
			// "resource":        resource,
			// "powerState":      powerState,
			"string(payload)": string(payload),
		}).Trace("Sent RF Telemetry event.")
	}

	return
}

func connectToVault() securestorage.SecureStorage {
	vaultAddr, ok := os.LookupEnv("VAULT_ADDR")
	if !ok {
		// If no address provided don't use Vault.
		return nil
	}

	// Setup Vault. It's kind of a big deal, so we'll wait forever for this to work.
	log.WithField("vaultAddr", vaultAddr).Info("Connecting to Vault...")
	var secureStorage securestorage.SecureStorage
	for {
		// Start a connection to Vault
		var err error
		if secureStorage, err = securestorage.NewVaultAdapter(""); err == nil {
			log.WithFields(log.Fields{
				"vaultAddr": vaultAddr,
			}).Info("Connected to Vault")
			break
		}

		log.WithFields(log.Fields{
			"vaultAddr": vaultAddr,
			"err":       err,
		}).Error("Unable to connect to Vault! Trying again in 1 second...")
		time.Sleep(1 * time.Second)
	}

	return secureStorage
}

func setupCompCredsVault(secureStorage securestorage.SecureStorage) bool {
	vaultKeypath, ok := os.LookupEnv("HMS_VAULT_KEYPATH")
	if !ok {
		log.Warning("Unable to setup compcreds because HMS_VAULT_KEYPATH isn't set.")
		return false
	}

	compCredStore = *compcredentials.NewCompCredStore(vaultKeypath, secureStorage)
	return true
}

func addXNameService(namespace string, xname string) (created bool, err error) {
	// Set up the in-cluster config...
	config, err := rest.InClusterConfig()
	if err != nil {
		// Errors may or may not be interesting.  If running
		// stand-alone, they definitely are not.  If running
		// in K8s, they are more interesting, but we will get
		// another shot at this if anything changes, so
		// possibly less of a problem.	Log it and return it.
		log.WithFields(log.Fields{"xname": xname, "err": err}).Warning("failed to set up in-cluster K8s config to create service endpoint for xname")
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("It is safe to ignore warnings about in-cluster K8s configs when not running under K8s...")
		return
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.WithFields(log.Fields{"xname": xname, "err": err}).Warning("failed to create K8s client to create service endpoint for xname")
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("It is safe to ignore warnings about K8s clients (though odd you would get there) when not running under K8s...")
		return
	}
	// Try to get the service. If it already exists, we are done here...
	_, err = clientset.CoreV1().Services(namespace).Get(xname, metav1.GetOptions{})
	if err == nil {
		log.WithFields(log.Fields{"namespace": namespace, "xname": xname}).Debug("service for xname already exists, no need to add it")
		created = false
		return
	}
	// Probably not there (log the error just in case), construct and add the service...
	log.WithFields(log.Fields{"namespace": namespace, "xname": xname, "err": err}).Debug("service for probably doesn't exist (check error) adding it now")
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      xname,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/instance": "cray-hms-rts",
				"app.kubernetes.io/name":     "cray-hms-rts",
			},
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name:       "https",
					Protocol:   "TCP",
					Port:       8083,
					TargetPort: intstr.FromInt(8083),
				},
				corev1.ServicePort{
					Name:       "http",
					Protocol:   "TCP",
					Port:       8082,
					TargetPort: intstr.FromInt(8082),
				},
			},
		},
	}
	_, err = clientset.CoreV1().Services(namespace).Create(service)
	if err != nil {
		log.WithFields(log.Fields{"xname": xname, "err": err}).Warning("failed to create K8s service endpoint for xname")
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("It is safe to ignore warnings about K8s service endpoints (though odd you would get there) when not running under K8s...")
		return
	}
	created = true
	return
}

func checkReady(ctx context.Context, xname string) (err error) {
	// Get redfish endpoint credentials from Vault
	cred, err := compCredStore.GetCompCred(xname)
	if err != nil {
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("Unable to retrieve credentials to check endpoint readiness")
		return err
	}

	// Do some validation, this might help with debugging problems
	// later, and it is a good sanity check in any case.
	if len(cred.Username) < 1 {
		err = errors.New("zero length username in credentials, not checking readiness")
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("Bad vault credentials (username)")
		return
	}
	if len(cred.Password) < 1 {
		err = errors.New("zero length password in credentials, not checking readiness")
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("Bad vault credentials (password)")
		return
	}
	if cred.Xname != xname {
		err = errors.New(fmt.Sprintf("Xname found in credentials ['%s'] not a match, not checking readiness", cred.Xname))
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("Bad credentials (xname)")
		return
	}
	if len(cred.URL) < 1 {
		err = errors.New("zero length URL in credentials, not checking readiness")
		log.WithFields(log.Fields{"xname": xname, "err": err}).Info("Bad credentials (URL)")
		return
	}

	// It all looks good, try the request and see if we are ready yet.
	var path string = "https://" + cred.URL + "/redfish/v1"
	req := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusOK},
		FullURL:             path,
		Method:              "GET",
		CustomHeaders:       getSvcInstName(),
		Auth: &hmshttp.Auth{
			Username: cred.Username,
			Password: cred.Password,
		},
	}
	_, _, err = req.DoHTTPAction()
	if err == nil {
		log.WithField("xname", xname).Debug("Redfish endpoint is ready")
	}
	// Ready or not, we know why now, so just return.
	return
}

// XNameSet is a thread-safe data structure that represents a set of xnames.
type XNameSet struct {
	mux sync.Mutex
	m   map[string]bool
}

func NewXNameSet() *XNameSet {
	return &XNameSet{
		m: map[string]bool{},
	}
}

func (s *XNameSet) Get(xname string) bool {
	s.mux.Lock()
	defer s.mux.Unlock()

	return s.m[xname]
}

func (s *XNameSet) Set(xname string, value bool) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.m[xname] = value
}
