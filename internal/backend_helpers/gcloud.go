// MIT License
//
// (C) Copyright 2020-2023 Hewlett Packard Enterprise Development LP
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
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/rts_credential_store"

	"cloud.google.com/go/compute/metadata"
	compcredentials "github.com/Cray-HPE/hms-compcredentials"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

// Generic helper functions

func translateGCloudStatusToRF(status string) (rfStatus string) {
	switch status {
	case "RUNNING":
		rfStatus = "On"
	case "PROVISIONING", "STAGING":
		rfStatus = "PoweringOn"
	case "STOPPING", "SUSPENDING":
		rfStatus = "PoweringOff"
	case "STOPPED", "SUSPENDED", "TERMINATED":
		rfStatus = "Off"
	default:
		rfStatus = "UNKNOWN"
	}

	return
}

func getProjectID() (project string, err error) {
	project, ok := os.LookupEnv("GCLOUD_PROJECT")
	if ok {
		return
	}
	// Not found in the environment, so we are going to
	// query for it.
	//
	// The following loop will wait forever, since there is
	// nothing useful we can do if this doesn't resolve itself,
	// and it generally does resolve itself fairly quickly.
	for project, err = metadata.ProjectID(); err != nil; project, err = metadata.ProjectID() {
		log.WithField("err", err).Info("Failed to get project ID from GCP, retrying in 10 seconds")
		time.Sleep(10 * time.Second)
	}
	log.WithField("project", project).Info("Got project id from GCP")
	return
}

// Interface specific functions

func (helper *GCloudHelper) findInstanceByXname(ctx context.Context, xname string) (*compute.Instance, error) {
	var instance *compute.Instance
	instance = nil
	found := false
	req := helper.computeService.Instances.AggregatedList(helper.project).Filter("labels.xname = " + xname)
	err := req.Pages(ctx, func(page *compute.InstanceAggregatedList) error {
		for _, instancesScopedList := range page.Items {
			for _, instance = range instancesScopedList.Instances {
				// There will be only one instance
				// with the given xname, so all we are
				// doing is looking through the zones
				// to find it.	When we get here, we
				// have found an instance, which is
				// it.	So, pick it off and terminate
				// the loops.
				found = true
				break
			}

			if found {
				break
			}
		}
		// No way to know whether this page was supposed to
		// hold the instance, so just return nil.  We will
		// error in the outer function if the thing wasn't
		// found.
		return nil
	})
	if err != nil {
		log.WithField("xname", xname).Error(err)
		return nil, err
	}
	if !found {
		err = errors.New("no instance with specified xname was found.")
		log.WithField("xname", xname).Error(err)
		return nil, err
	}
	return instance, err
}

func (helper *GCloudHelper) initSystem(xname string, instance *compute.Instance) (err error) {
	systemKeyspace, _ := helper.RedisHelper.addMemberToSet(filepath.Join(xname, ServiceRootKeyspace), "Systems")
	helper.RedisHelper.setValueForPropertyInKeyspace(systemKeyspace, "Name", "Computer System Collection")
	helper.RedisHelper.addMemberToSet(systemKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(systemKeyspace, "@odata.type")
	helper.RedisHelper.addMemberToSet(systemKeyspace, "Members")

	// Add this instance to the Members of the Systems for this xname.
	thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(systemKeyspace, "Members")
	if addErr != nil {
		err = fmt.Errorf("unable to add member to array for instance: %s", xname)
		return
	}

	// Generic "Self" URL for all instances.
	newInstanceKeyspace := systemKeyspace + "/Self"
	helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", newInstanceKeyspace)

	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.type")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Name", "Computer System Chassis")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Id", "Self")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Manufacturer", "Cray, Inc.")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Model", "Google Cloud")
	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "PowerState")

	// Actions
	actionsKeyspace, addErr := helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "Actions")
	if addErr != nil {
		err = fmt.Errorf("unable to add Actions to instance %s", xname)
		return
	}
	actionName := "ComputerSystem.Reset"
	computerSystemResetKeyspace, addErr := helper.RedisHelper.addMemberToSet(actionsKeyspace, "#"+actionName)
	computerSystemResetTargetURI := filepath.Join(actionsKeyspace, actionName)
	helper.RedisHelper.setValueForPropertyInKeyspace(computerSystemResetKeyspace, "target",
		computerSystemResetTargetURI)

	allowableValuesKeyspace, addErr := helper.RedisHelper.addMemberToSet(computerSystemResetKeyspace,
		"ResetType@Redfish.AllowableValues")
	if addErr != nil {
		err = fmt.Errorf("unable to add AllowableValues to ResetType for xname %s", xname)
		return
	}
	helper.RedisHelper.Redis.RPush(allowableValuesKeyspace, "On", "GracefulShutdown", "ForceRestart")

	return
}

func (helper *GCloudHelper) initChassis(xname string, instance *compute.Instance) (err error) {
	chassisKeyspace, _ := helper.RedisHelper.addMemberToSet(filepath.Join(xname, ServiceRootKeyspace), "Chassis")
	helper.RedisHelper.setValueForPropertyInKeyspace(chassisKeyspace, "Name", "Chassis Collection")
	helper.RedisHelper.addMemberToSet(chassisKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(chassisKeyspace, "@odata.type")
	helper.RedisHelper.addMemberToSet(chassisKeyspace, "Members")

	// Add this instance to the Members of the Chassis for this xname.
	thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(chassisKeyspace, "Members")
	if addErr != nil {
		err = fmt.Errorf("unable to add member to array for instance: %s", xname)
		return
	}

	// Generic "Self" URL for all instances.
	newInstanceKeyspace := chassisKeyspace + "/Self"
	helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", newInstanceKeyspace)

	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.type")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Name", "Computer System Chassis")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Id", "Self")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Manufacturer", "Cray, Inc.")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Model", "Google Cloud")

	linksKeyspace, _ := helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "Links")
	systemsKeyspace, _ := helper.RedisHelper.addMemberToArray(linksKeyspace, "ComputerSystems")
	helper.RedisHelper.setValueForPropertyInKeyspace(systemsKeyspace, "@odata.id",
		filepath.Join(SystemsKeyspace, "Self"))

	return
}

func (helper *GCloudHelper) initManagers(xname string, instance *compute.Instance) (err error) {
	// Initialize the Manager Collection
	err = helper.RedisHelper.initManagerCollection(xname)
	if err != nil {
		return fmt.Errorf("unable to init manager collection for %s: %w", xname, err)
	}

	// Initialize a Manager instance
	err = helper.RedisHelper.initManager(xname, "RTS", "Google Cloud")
	if err != nil {
		return fmt.Errorf("unable to init manager for %s: %w", xname, err)
	}

	return
}

func (helper *GCloudHelper) initInstance(ctx context.Context, instance *compute.Instance) (err error) {
	xname := instance.Labels["xname"]

	logFields := log.Fields{
		"xname":    xname,
		"instance": fmt.Sprintf("%+v", instance),
	}

	// Find out if we are waiting for this XName endpoint in
	// Redfish to be ready for traffic, and how many times we have
	// checked that.  0 means this is the first time we have tried
	// to initialize this instance (not found means, for some
	// reason, we have forgotten an already initialized instance
	// and are re-initializing it).
	log.WithField("xname", xname).Trace("looking for xname in ready wait map")
	count, found := helper.waitReady[xname]
	if found {
		log.WithFields(log.Fields{"xname": xname, "count": count}).Trace("found xname in ready wait map")
	} else {
		log.WithField("xname", xname).Trace("did not find xname in ready wait map")
	}

	if !found || count == 0 {
		// Either we are re-initializing an already set up
		// instance or we are initializing this instance for
		// the first time ever.
		//
		//First make sure there aren't any keys for this xname
		removeErr := helper.RedisHelper.removeXnamePrependedKeys(xname)
		if removeErr != nil {
			err = fmt.Errorf("unable to remove keys from Redis: %s", removeErr)
			return
		}

		// Generic keys that need to be in place for every instnace.
		rootKeyspace, _ := helper.RedisHelper.addMemberToSet("/redfish", "v1")
		helper.RedisHelper.setValueForPropertyInKeyspace(rootKeyspace, "Name", "Service Root")
		helper.RedisHelper.setValueForPropertyInKeyspace(rootKeyspace, "Description", "The Redfish ServiceRoot")
		helper.RedisHelper.addMemberToSet(rootKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(rootKeyspace, "@odata.type")

		versionStr, ok := os.LookupEnv("SCHEMA_VERSION")
		if ok {
			helper.RedisHelper.setValueForPropertyInKeyspace(rootKeyspace, "RedfishVersion", versionStr)
		}

		// These are the "root" members. From here the code will then look up everything specific to the xname.
		helper.RedisHelper.addMemberToSet(rootKeyspace, "Chassis")
		helper.RedisHelper.addMemberToSet(rootKeyspace, "Systems")
		helper.RedisHelper.addMemberToSet(rootKeyspace, "Managers")

		// Create a new pipeline
		helper.RedisHelper.RedisActivePipeline = helper.RedisHelper.Redis.Pipeline()

		initFunctions := [...]func(string, *compute.Instance) error{
			helper.initChassis,
			helper.initSystem,
			helper.initManagers,
		}

		for _, initFunction := range initFunctions {
			logFields["initFunction"] = runtime.FuncForPC(reflect.ValueOf(initFunction).Pointer()).Name()

			err = initFunction(xname, instance)
			if err != nil {
				logFields["err"] = err
				log.WithFields(logFields).Error("Initialization function failed")

				return
			} else {
				log.WithFields(logFields).Debug("Initialization function succeeded")
			}
		}

		// Dump this pipeline.
		_, err = helper.RedisHelper.RedisActivePipeline.Exec()
		helper.RedisHelper.RedisActivePipeline = nil
		if err != nil {
			log.WithFields(logFields).Error("Unable to Exec() initInstance pipeline")
			return
		}

		// Setup Certificate Service for the device
		err = helper.CertificateService.InitForXName(xname)
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Error("Failed to initialize Certificate Service for device")
			return
		}

		log.WithFields(logFields).Debug("Instance initialized")

		creds, err := helper.RTSCredentialStore.GetGlobalCredentials()
		if err != nil {
			log.WithField("err", err).Fatal("Unable to get default credentails for RTS")
		}

		// Add the credentials to talk to RTS to Vault so HSM
		// knows how to handle it.
		cred := compcredentials.CompCredentials{
			Xname:    xname,
			URL:      xname + ":8083",
			Username: creds.Username,
			Password: creds.Password,
		}

		compErr := compCredStore.StoreCompCred(cred)
		if compErr != nil {
			err = fmt.Errorf("unable to store credentials in Vault: %s", compErr)
			log.Error(err)
			return err
		} else {
			log.WithField("xname", xname).Debug("Added credentials to Vault")
		}
	}
	// Check whether the XName endpoint in Redfish is ready to
	// receive requests.  If not, we aren't ready to let HSM know
	// about this endpoint, but we will be back...
	if found {
		log.WithFields(logFields).Debug("Checking Redfish endpoint readiness")
		err = checkReady(ctx, xname)
		if err != nil {
			helper.waitReady[xname] += 1
			msg := "redfish endpoint readiness check failed (will retry)"
			if count < 10 {
				log.WithFields(log.Fields{"xname": xname, "err": err, "count": count}).Debug(msg)
			} else {
				// Escalate to a warning if it seems to be persistent...
				log.WithFields(log.Fields{"xname": xname, "err": err, "count": count}).Warning(msg)
			}
			// Not ready yet, no error, just return.
			err = nil
			return
		}
		log.WithFields(logFields).Debug("Redfish endpoint is ready")
		delete(helper.waitReady, xname)
	}

	// Now that all the necessary data is in Redis tell HSM we
	// exist and have it discover us.  This may not work the first
	// time, since HSM takes a while to come up, so give it a good
	// try and then come back for it later if it doesn't work.
	_, found = helper.informHSM[xname]
	if found {
		log.WithFields(log.Fields{"xname": xname, "count": count}).Trace("found xname in inform HSM map")
		hsmErr := informHSM(ctx, xname)
		if hsmErr != nil {
			log.WithFields(log.Fields{"xname": xname, "hsmErr": hsmErr}).Info("failed to inform HSM (will try again later)")
			helper.informHSM[xname] += 1
		} else {
			log.WithField("xname", xname).Debug("Informed HSM")
			delete(helper.informHSM, xname)
		}
	} else {
		log.WithField("xname", xname).Trace("did not find xname in inform HSM map")
	}
	return
}

func (helper *GCloudHelper) updateKnownInstance(ctx context.Context, instance *compute.Instance) {
	// Check to see if the status has changed.
	xname, hasXname := instance.Labels["xname"]
	if !hasXname {
		// Looking at a node with no xname (generally the PIT on vShasta)
		log.WithFields(log.Fields{
			"instance.Name": instance.Name,
		}).Debug("instance has no xname")
		return
	}
	helper.KnownInstancesLock.Lock()
	defer helper.KnownInstancesLock.Unlock()
	knownInstance, instanceKnown := helper.KnownInstances[xname]
	_, waitReady := helper.waitReady[xname]
	_, informHSM := helper.informHSM[xname]
	if !instanceKnown || waitReady || informHSM {
		// Add this instance to the list of known. We will do that unconditionally after the
		// else clause, but do the stuff we need to set that up here...
		log.WithFields(log.Fields{
			"xname":           xname,
			"instance.Status": instance.Status,
		}).Info("Initializing new instance")
		helper.initInstance(ctx, instance)
	} else {
		log.WithFields(log.Fields{
			"xname":                xname,
			"instance.Status":      instance.Status,
			"knownInstance.status": knownInstance.Status,
		}).Info("Updating known instance")

		creds, err := helper.RTSCredentialStore.GetGlobalCredentials()
		if err != nil {
			log.WithField("err", err).Fatal("Unable to get default credentials for RTS")
		}

		// Add the credentials to talk to RTS to Vault so HSM
		// knows how to handle it.
		cred := compcredentials.CompCredentials{
			Xname:    xname,
			URL:      xname + ":8083",
			Username: creds.Username,
			Password: creds.Password,
		}

		compErr := compCredStore.StoreCompCred(cred)
		if compErr != nil {
			err = fmt.Errorf("unable to store credentials in Vault: %s", compErr)
			log.Error(err)
		} else {
			log.WithField("xname", xname).Debug("Added credentials to Vault")
		}
		if knownInstance.Status != instance.Status {
			// If the status has changed, send a RF event.
			instanceStatusRedfish := translateGCloudStatusToRF(instance.Status)
			_ = postRFPowerEvent(ctx, xname,
				filepath.Join(SystemsKeyspace, "Self"),
				instanceStatusRedfish)

			log.WithFields(log.Fields{
				"knownInstance.Status": knownInstance.Status,
				"instance.Status":      instance.Status,
				"xname":                xname,
			}).Info("Known instance has changed status")
		}
	}
	helper.KnownInstances[xname] = instance
}

func (helper *GCloudHelper) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	// Create a map of xnames that we found on this iteration. Use a map because it is easy to search later for
	// missing instances, all entries will be 'true'.
	seen := map[string]bool{}
	req := helper.computeService.Instances.AggregatedList(helper.project)
	if err := req.Pages(ctx, func(page *compute.InstanceAggregatedList) error {
		for _, instancesScopedList := range page.Items {
			for _, instance := range instancesScopedList.Instances {
				xname, hasXname := instance.Labels["xname"]
				if !hasXname {
					log.WithField("instanceName", instance.Name).Debug("Instance has no xname, skipped...")
					continue
				}
				// Make sure the xname resolves before anything else.  If the service has already been
				// added, this does nothing, if not it adds it.  If it fails for some reason, there is
				// nothing we can do, but we we will try again next time through, so it should resolve
				// eventually.
				created, err := addXNameService(helper.namespace, xname)
				if err != nil {
					log.WithFields(log.Fields{"xname": xname, "namespace": helper.namespace, "err": err}).Warning("failed to add xname service")
					continue
				}
				if created {
					// Set up to wait for the redfish endpoint for this xname to be ready and
					// responsive. The value is a retry counter to allow for warnings if it is
					// taking too long.  The entry will be removed from the map when the endpoint
					// becomes responsive. This gives the new service a chance to be up and ready
					// and istio or whatever a chance to recognize the new services.
					log.WithField("xname", xname).Trace("adding xname to ready wait map")
					helper.waitReady[xname] = 0
					helper.informHSM[xname] = 0
				}

				// Note the discovery for later to prevent removal from known instances
				seen[xname] = true
				// Process the instance...
				helper.updateKnownInstance(ctx, instance)
			}
		}
		return nil
	}); err != nil {
		log.WithField("err", err).Info("Node discovery failed, trying again later")
		return err
	}
	// Search for nodes that have gone away since the last time we looked.
	//
	// Holding the lock over the whole loop to make sure we don't collide with
	// another instance of ourself. Defer the unlock to keep the logic contiguous
	// and harder to break
	helper.KnownInstancesLock.Lock()
	defer helper.KnownInstancesLock.Unlock()
	for k, _ := range helper.KnownInstances {
		if _, found := seen[k]; !found {
			// This instance has disappeared, remove it
			// from HSM, and forget about it.
			log.WithField("xname", k).Debug("xname has disappeared, removing it from HSM")
			delete(helper.KnownInstances, k)
			_ = deleteFromHSM(ctx, k)
		}
	}
	return
}

func (helper *GCloudHelper) SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error) {
	project, err := getProjectID()
	if err != nil {
		log.Error(err.Error()) // capture why it failed
		err = errors.New("GCloud Project ID can't be determined, use GCLOUD_PROJECT environment variable to set it explicitly")
		return
	}
	helper.project = project

	// The namespace where the xname services will be registered.  Defaults to "services" but can be configured in
	// the environment.
	helper.namespace = "services"
	namespace, ok := os.LookupEnv("XNAME_NAMESPACE")
	if ok {
		helper.namespace = namespace
	}

	// Make waitReady empty not nil
	helper.waitReady = map[string]int{}

	// Make informHSM empty not nil
	helper.informHSM = map[string]int{}

	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport
	httpClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	computeService, serviceErr := compute.NewService(ctx)
	if serviceErr != nil {
		err = fmt.Errorf("unable to setup new compute service: %s", serviceErr)
		log.Fatal(err)
		return
	}
	helper.computeService = computeService

	// Time to setup the connection to Vault
	jawsVaultKeypath, ok := os.LookupEnv("JAWS_VAULT_KEYPATH")
	if !ok {
		log.Fatal("Environment variable JAWS_VAULT_KEYPATH was not set")
	}

	secureStorage := connectToVault()
	if secureStorage == nil {
		log.Fatal("Unable to connect to Vault")
	}
	if ok := setupCompCredsVault(secureStorage); !ok {
		log.Fatal("Failed to setup CompCredsVault")
	}

	// Certificate Service setup
	err = helper.CertificateService.SetupCertificateService("RTS", GenericCertificateID)
	if err != nil {
		log.WithError(err).Error("Failed to setup certificate service")
		return
	}

	// important: the CREDS here VVV MUST 100% match the creds in initInstance; there can ONLY currently be 1 cred for all BMCs
	/* When RTS stands up the SetupBackendHelper will get called first; we will then set the global password into memory and
	* set the account password (initAccount).  then when initInstance is called it will FLUSH creds into vault that match
	* the global password.  Every time RTS restarts the account service pw and the vault pw's will get reset. So if RTS has
	* to be reset it would be like the BMC password being reset; but this is how we get randomness.
	 */
	helper.RTSCredentialStore = rts_credential_store.NewRTSCredStore(jawsVaultKeypath, secureStorage)

	if ok := setupCompCredsVault(secureStorage); !ok {
		log.Fatal("Failed to setup CompCredsVault")
	}

	// Get default credentails from vault
	creds, err := helper.RTSCredentialStore.GetGlobalCredentials()
	if err != nil {
		log.WithField("err", err).Fatal("Unable to get default credentails for RTS")
	}

	password := creds.Password
	username := creds.Username
	role := "Administrator"
	err = helper.RedisHelper.initAccount(username, role, password)
	if err != nil {
		log.WithFields(log.Fields{
			"error":    err,
			"username": username,
			"role":     role,
		}).Error("problem encountered initializing account")
	}

	return
}

func (helper *GCloudHelper) GetEnvForXname(xname string) (env map[string]string, err error) {
	env = map[string]string{}

	helper.KnownInstancesLock.Lock()
	defer helper.KnownInstancesLock.Unlock()
	if device, ok := helper.KnownInstances[xname]; ok {
		env["RTS_XNAME"] = device.Labels["xname"]
		return
	}

	err = errors.New("unable to find xname in instance list")
	log.WithField("xname", xname).Error(err)
	return
}

func (helper *GCloudHelper) RunBackendHelper(ctx context.Context, key string, args []string,
	env map[string]string) (value string, err error) {

	xname, ok := env["RTS_XNAME"]
	if !ok {
		err = errors.New("RTS_XNAME not in environment variables")
		log.Error(err.Error())
		return
	}
	log.WithFields(log.Fields{
		"key":   key,
		"xname": xname,
		"args":  fmt.Sprintf("%+v", args),
		"time":  time.Now().String(),
	}).Info("ERIC: entering RunBackGroundHelper")

	// Check to make sure we actually know about this device and
	// get the instance for use later.
	log.WithFields(log.Fields{
		"key":   key,
		"xname": xname,
		"args":  fmt.Sprintf("%+v", args),
		"time":  time.Now().String(),
	}).Info("ERIC: taking the instances lock")
	helper.KnownInstancesLock.Lock()
	instance, deviceIsKnown := helper.KnownInstances[xname]
	helper.KnownInstancesLock.Unlock()
	log.WithFields(log.Fields{
		"key":   key,
		"xname": xname,
		"args":  fmt.Sprintf("%+v", args),
		"time":  time.Now().String(),
	}).Info("ERIC: released the instances lock")
	if !deviceIsKnown {
		err = fmt.Errorf("unknown xname: %s", xname)
		log.Error(err.Error())
		return
	}

	// Do some sanity checks to make sure we're getting what we expect.
	// Every key that comes in could be xname prefixed, use a regular expression to capture that.
	keyRegex := regexp.MustCompile(`(x[0-9]{1,4}c[0-7]s[0-9]+b[0-9]+)*(` + ServiceRootKeyspace + `\S+$)`)
	keyMatches := keyRegex.FindStringSubmatch(key)

	var fullKey string
	switch len(keyMatches) {
	case 2:
		// Not xname prefixed
		fullKey = keyMatches[1]
	case 3:
		// Xname prefixed
		fullKey = keyMatches[2]
	default:
		err = errors.New("key format not what expected")
		log.WithField("key", key).Error("Unable to run regex expression on key")
		return
	}

	strippedKey := strings.TrimPrefix(fullKey, ServiceRootKeyspace)
	// If the trim didn't work it will be the same as the key and we don't want that.
	if strippedKey == key {
		err = errors.New("unexpected key prefix")
		log.WithField(
			"instancePrefixedKey", strippedKey,
		).Error("Unable to derive the stripped key")
		return
	}

	log.Debug("serving request '" + fullKey + "' from gcloud backend, xname = '" + xname + "'")
	// There are better ways to do this, but for now I'm just hard coding all the possible keys to their desired thing.
	if strippedKey == "/Systems/Self/Actions/ComputerSystem.Reset" {
		// Extract the zone and name from the known instance
		zoneRegex := regexp.MustCompile(`[^/]*$`)
		zone := string(zoneRegex.Find([]byte(instance.Zone)))
		name := instance.Name

		desiredPowerState, ok := env["ResetType"]
		if !ok {
			err = errors.New("ResetType action requested, but ResetType not provided")
			log.Error(err.Error())

			return
		}

		log.WithFields(log.Fields{
			"key":               key,
			"xname":             xname,
			"desiredPowerState": desiredPowerState,
			"args":              fmt.Sprintf("%+v", args),
			"time":              time.Now().String(),
		}).Info("ERIC: reset operation")
		switch desiredPowerState {
		case "On":
			_, err = helper.computeService.Instances.Start(helper.project, zone, name).Context(ctx).Do()
		case "GracefulShutdown":
			_, err = helper.computeService.Instances.Stop(helper.project, zone, name).Context(ctx).Do()
		case "ForceRestart":
			_, err = helper.computeService.Instances.Reset(helper.project, zone, name).Context(ctx).Do()
		default:
			log.WithField("desiredPowerState", desiredPowerState).Error("Unknown desired power state")
			return
		}

		// Make sure to remove any cached keys relating to power state so the next query forces a fresh load.
		invalidatedKey := filepath.Join(xname, SystemsKeyspace, "Self", "PowerState")
		helper.RedisHelper.invalidateRedisKeys([]string{invalidatedKey})
		log.WithFields(log.Fields{
			"key":               key,
			"xname":             xname,
			"desiredPowerState": desiredPowerState,
			"args":              fmt.Sprintf("%+v", args),
			"time":              time.Now().String(),
		}).Info("ERIC: reset operation completed")
		return
	} else if strippedKey == "/Systems/Self/PowerState" {
		// Make sure the metadata is up to date for this instance.
		instance, err = helper.findInstanceByXname(ctx, xname)
		helper.updateKnownInstance(ctx, instance)
		log.WithFields(log.Fields{
			"key":   key,
			"xname": xname,
			"args":  fmt.Sprintf("%+v", args),
			"time":  time.Now().String(),
		}).Info("ERIC: status query")

		// The Gcloud states don't map perfectly to Redfish ones, do that translation here.
		// The legal states are: On, Off, PoweringOn, and PoweringOff
		value = translateGCloudStatusToRF(instance.Status)
		log.WithFields(log.Fields{
			"key":    key,
			"xname":  xname,
			"status": value,
			"args":   fmt.Sprintf("%+v", args),
			"time":   time.Now().String(),
		}).Info("ERIC: status query completed")

		return
	}

	value, err = helper.RedisHelper.Redis.Get(key).Result()
	log.WithFields(log.Fields{
		"key":    key,
		"xname":  xname,
		"status": value,
		"args":   fmt.Sprintf("%+v", args),
		"value":  value,
		"time":   time.Now().String(),
		"err":    err,
	}).Info("ERIC: leaving RunBackgroundHelper")
	return
}
