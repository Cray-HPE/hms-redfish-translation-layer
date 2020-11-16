// Copyright 2020 Hewlett Packard Enterprise Development LP

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
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
	compcredentials "stash.us.cray.com/HMS/hms-compcredentials"
)

var (
	RedfishUsername string
	RedfishPassword string
)

// Interface specific functions

func (helper *RFSimulatorHelper) initSystem(xname string) (err error) {
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
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Model", "Redfish Simulator")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "PowerState", "On")

	processorSummaryKeyspace, _ := helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "ProcessorSummary")
	helper.RedisHelper.setValueForPropertyInKeyspace(processorSummaryKeyspace, "Count",
		fmt.Sprintf("%d", runtime.NumCPU()))
	helper.RedisHelper.setValueForPropertyInKeyspace(processorSummaryKeyspace, "Model", "Simulated")

	memorySummaryKeyspace, _ := helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "MemorySummary")
	helper.RedisHelper.setValueForPropertyInKeyspace(memorySummaryKeyspace, "TotalSystemMemoryGiB", "42")

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

func (helper *RFSimulatorHelper) initChassis(xname string) (err error) {
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
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Model", "Redfish Simulator")

	linksKeyspace, _ := helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "Links")
	systemsKeyspace, _ := helper.RedisHelper.addMemberToArray(linksKeyspace, "ComputerSystems")
	helper.RedisHelper.setValueForPropertyInKeyspace(systemsKeyspace, "@odata.id",
		filepath.Join(SystemsKeyspace, "Self"))

	return
}

func (helper *RFSimulatorHelper) initManagers(xname string) (err error) {
	// Initialize the Manager Collection
	err = helper.RedisHelper.initManagerCollection(xname)
	if err != nil {
		return fmt.Errorf("unable to init manager collection for %s: %w", xname, err)
	}

	// Initialize a Manager instance
	err = helper.RedisHelper.initManager(xname, "RTS", "Redfish Simulator")
	if err != nil {
		return fmt.Errorf("unable to init manager for %s: %w", xname, err)
	}

	return
}

func (helper *RFSimulatorHelper) populateFirmwareInventory(updateKeyspace string) (err error) {
	firmwareInventoryKeyspace, _ := helper.RedisHelper.addMemberToSet(updateKeyspace, "FirmwareInventory")
	helper.RedisHelper.addMemberToSet(firmwareInventoryKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(firmwareInventoryKeyspace, "@odata.type")
	helper.RedisHelper.setValueForPropertyInKeyspace(firmwareInventoryKeyspace, "Description",
		"Collection of Firmware Inventory resources available to the UpdateService")
	helper.RedisHelper.setValueForPropertyInKeyspace(firmwareInventoryKeyspace, "Name",
		"Firmware Inventory Collection")

	// These are all the things that can be FW updated
	members := []string{"BIOS", "BMC"}
	for _, member := range members {
		// Just start all of them the same, it can change later as requests come in.
		thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(firmwareInventoryKeyspace, "Members")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for keyspace: %s", updateKeyspace)
			return
		}

		newInstanceKeyspace := firmwareInventoryKeyspace + "/" + member
		helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", newInstanceKeyspace)

		helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.type")
		helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Description",
			fmt.Sprintf("%s Firmware Inventory", member))
		helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Id", member)
		helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Name", member)
		helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Updateable", "True")
		helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Version", "1")
	}

	return
}

func (helper *RFSimulatorHelper) initUpdateService(xname string) (err error) {
	updateKeyspace, _ := helper.RedisHelper.addMemberToSet(filepath.Join(xname, ServiceRootKeyspace), "UpdateService")
	helper.RedisHelper.setValueForPropertyInKeyspace(updateKeyspace, "Name", "Update Service")
	helper.RedisHelper.setValueForPropertyInKeyspace(updateKeyspace, "Id", "Update Service")
	helper.RedisHelper.addMemberToSet(updateKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(updateKeyspace, "@odata.type")
	helper.RedisHelper.setValueForPropertyInKeyspace(updateKeyspace, "Description", "Redfish Update Service")
	helper.RedisHelper.setValueForPropertyInKeyspace(updateKeyspace, "ServiceEnabled", "true")

	// Add the FW inventory pieces in a separate function
	helper.populateFirmwareInventory(updateKeyspace)

	statusKeyspace, _ := helper.RedisHelper.addMemberToSet(updateKeyspace, "Status")
	helper.RedisHelper.setValueForPropertyInKeyspace(statusKeyspace, "Health", "OK")
	helper.RedisHelper.setValueForPropertyInKeyspace(statusKeyspace, "State", "Enabled")

	// Actions
	actionsKeyspace, addErr := helper.RedisHelper.addMemberToSet(updateKeyspace, "Actions")
	if addErr != nil {
		err = fmt.Errorf("unable to add Actions to instance %s", xname)
		return
	}
	actionName := "UpdateService.SimpleUpdate"
	simpleUpdateKeyspace, addErr := helper.RedisHelper.addMemberToSet(actionsKeyspace, "#"+actionName)
	simpleUpdateTargetURI := filepath.Join(actionsKeyspace, actionName)
	helper.RedisHelper.setValueForPropertyInKeyspace(simpleUpdateKeyspace, "target",
		simpleUpdateTargetURI)

	return
}

func (helper *RFSimulatorHelper) initSimulator(ctx context.Context, xname string) (err error) {
	logFields := log.Fields{
		"xname": xname,
	}

	// First make sure there aren't any keys for this xname
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
	helper.RedisHelper.addMemberToSet(rootKeyspace, "UpdateService")

	// Create a new pipeline
	helper.RedisHelper.RedisActivePipeline = helper.RedisHelper.Redis.Pipeline()

	initFunctions := [...]func(string) error{
		helper.initChassis,
		helper.initSystem,
		helper.initManagers,
		helper.initUpdateService,
	}

	for _, initFunction := range initFunctions {
		logFields["initFunction"] = runtime.FuncForPC(reflect.ValueOf(initFunction).Pointer()).Name()

		err = initFunction(xname)
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Error("Initialization function failed")

			return
		} else {
			log.WithFields(logFields).Debug("Initialization function succeeded")
		}
	}

	// Setup Certificate Service for the device
	err = helper.CertificateService.InitForXName(xname)
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Failed to initialize Certificate Service for device")
		return
	}

	// Dump this pipeline.
	_, err = helper.RedisHelper.RedisActivePipeline.Exec()
	helper.RedisHelper.RedisActivePipeline = nil
	if err != nil {
		log.WithFields(logFields).Error("Unable to Exec() initInstance pipeline")
		return
	}

	log.WithFields(logFields).Debug("Instance initialized")

	// Add the credentials to talk to RTS to Vault so HSM
	// knows how to handle it.
	cred := compcredentials.CompCredentials{
		Xname:    xname,
		URL:      xname + ":8083",
		Username: RedfishUsername,
		Password: RedfishPassword,
	}
	err = compCredStore.StoreCompCred(cred)
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to store credentials in Vault")
		return
	} else {
		log.WithFields(logFields).Debug("Added credentials to Vault")
	}

	return
}

func (helper *RFSimulatorHelper) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	// Make sure HSM knows about these simulated endpoints.
	for xname, _ := range helper.KnownInstances {
		logFields := log.Fields{
			"xname": xname,
		}

		err = informHSM(ctx, xname)
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Error("Unable to inform HSM about this simulator")
			return
		} else {
			log.WithFields(logFields).Debug("HSM informed about simulator")
		}
	}

	return
}

func (helper *RFSimulatorHelper) SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error) {
	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport
	httpClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	secureStorage := connectToVault()
	if secureStorage == nil {
		log.Fatal("Unable to connect to Vault")
	}
	if ok := setupCompCredsVault(secureStorage); !ok {
		log.Fatal("Failed to setup CompCredsVault")
	}

	var ok bool
	RedfishUsername, ok = os.LookupEnv("RF_USERNAME")
	if !ok {
		panic("Value must be set for RF_USERNAME")
	}
	RedfishPassword, ok = os.LookupEnv("RF_PASSWORD")
	if !ok {
		panic("Value must be set for RF_PASSWORD")
	}
	role := "Administrator"
	err = helper.RedisHelper.initAccount(RedfishUsername, role, RedfishPassword)
	if err != nil {
		log.WithFields(log.Fields{
			"error":    err,
			"username": RedfishUsername,
			"role":     role,
		}).Error("Problem encountered initializing account")
	}

	// Certificate Service setup
	err = helper.CertificateService.SetupCertificateService("RTS", GenericCertificateID)
	if err != nil {
		log.WithError(err).Error("Failed to setup certificate service")
		return
	}

	// Initialize instances
	simulatorXnames, ok := os.LookupEnv("RF_SIMULATOR_XNAMES")
	if ok {
		splitXnames := strings.Split(simulatorXnames, ",")

		for _, xname := range splitXnames {
			helper.initSimulator(ctx, xname)

			var instance interface{}
			helper.KnownInstances[xname] = instance
		}
	}

	return
}

func (helper *RFSimulatorHelper) GetEnvForXname(xname string) (env map[string]string, err error) {
	env = map[string]string{}

	if _, ok := helper.KnownInstances[xname]; ok {
		env["RTS_XNAME"] = xname

		return
	}

	err = errors.New("unable to find xname in device list")
	log.WithField("xname", xname).Error(err)

	return
}

func (helper *RFSimulatorHelper) RunBackendHelper(ctx context.Context, key string, args []string,
	env map[string]string) (value string, err error) {
	xname, ok := env["RTS_XNAME"]
	if !ok {
		err = errors.New("RTS_XNAME not in environment variables")
		log.Error(err.Error())
		return
	}

	// Check to make sure we actually know about this device and
	// get the instance for use later.
	_, deviceIsKnown := helper.KnownInstances[xname]
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

	// Here are all the actions that need to be supported. The basic formula they all have in common is to look at the
	// URL and determine what you need to do based on that. Since this is just a simulator all of the state information
	// is pushed directly into Redis so the next time it is requested that is the value that's given.
	if strippedKey == "/Systems/Self/Actions/ComputerSystem.Reset" {
		desiredPowerState, ok := env["ResetType"]
		if !ok {
			err = errors.New("ResetType action requested, but ResetType not provided")
			log.Error(err.Error())

			return
		}

		powerStateKeyspace := filepath.Join(xname, SystemsKeyspace, "Self")

		// Just make sure the desired power state is a legal one.
		switch desiredPowerState {
		case "GracefulShutdown":
			helper.RedisHelper.setValueForPropertyInKeyspace(powerStateKeyspace, "PowerState",
				"Off")
			postRFPowerEvent(ctx, xname, filepath.Join(SystemsKeyspace, "Self"), "Off")
		case "ForceRestart", "On":
			helper.RedisHelper.setValueForPropertyInKeyspace(powerStateKeyspace, "PowerState",
				"On")
			postRFPowerEvent(ctx, xname, filepath.Join(SystemsKeyspace, "Self"), "On")
		default:
			log.WithField("desiredPowerState", desiredPowerState).Error("Unknown desired power state")
			return
		}

		return
	} else if strippedKey == "/UpdateService/Actions/UpdateService.SimpleUpdate" {
		imageURI, ok := env["ImageURI"]
		if !ok {
			err = errors.New("UpdateService.SimpleUpdate action requested, but ImageURI not provided")
			log.Error(err.Error())

			return
		}
		transferProtocol, ok := env["TransferProtocol"]
		if !ok {
			err = errors.New("UpdateService.SimpleUpdate action requested, but TransferProtocol not provided")
			log.Error(err.Error())

			return
		}
		user, _ := env["User"]
		password, _ := env["Password"]
		updateComponent, ok := env["UpdateComponent"]
		if !ok {
			err = errors.New("UpdateService.SimpleUpdate action requested, but UpdateComponent not provided")
			log.Error(err.Error())

			return
		}
		restBMCString, ok := env["ResetBMC"]
		var resetBMC bool
		if ok {
			var parseErr error
			resetBMC, parseErr = strconv.ParseBool(restBMCString)
			if parseErr != nil {
				err = fmt.Errorf("UpdateService.SimpleUpdate action requested, but ResetBMC not boolean type: %w",
					parseErr)
				log.Error(err.Error())

				return
			}
		}

		// Mmmmm....hacky!
		versionRegex := regexp.MustCompile(`.+(\d\.\d\.\d).+`)
		versionSubmatches := versionRegex.FindStringSubmatch(imageURI)

		if len(versionSubmatches) < 2 {
			err = fmt.Errorf("ImageURI doesn't have a version in its filename, giving up")
			log.Error(err.Error())

			return
		}

		version := versionSubmatches[1]
		keyspace := filepath.Join(xname, ServiceRootKeyspace, "UpdateService", "FirmwareInventory", updateComponent)
		helper.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Version", version)

		log.WithFields(log.Fields{
			"ImageURI":         imageURI,
			"TransferProtocol": transferProtocol,
			"User":             user,
			"Password":         password,
			"UpdateComponent":  updateComponent,
			"ResetBMC":         resetBMC,
		}).Debug("UpdateService.SimpleUpdate action called")

		return
	}

	value, err = helper.RedisHelper.Redis.Get(key).Result()

	return
}
