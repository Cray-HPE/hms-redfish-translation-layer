// MIT License
//
// (C) Copyright [2023-2024] Hewlett Packard Enterprise Development LP
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
	"sync"
	"time"

	base "github.com/Cray-HPE/hms-base"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/rts_credential_store"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/slsapi"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/snmp_utilities"
	"github.com/hashicorp/go-retryablehttp"
	log "github.com/sirupsen/logrus"
)

type SNMPDevice struct {
	Xname string
	SNMP snmp_utilities.SNMPInterface
}

func (helper *SNMPSwitchHelper) initChassis(xname string, switchInfo snmp_utilities.EntityPhysicalTable) (err error) {
	chassisKeyspace, _ := helper.RedisHelper.addMemberToSet(filepath.Join(xname, ServiceRootKeyspace), "Chassis")
	helper.RedisHelper.setValueForPropertyInKeyspace(chassisKeyspace, "Name", "Chassis Collection")
	helper.RedisHelper.addMemberToSet(chassisKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(chassisKeyspace, "@odata.type")
	helper.RedisHelper.addMemberToSet(chassisKeyspace, "Members")

	// Add this device to the Members of the Chassis for this xname.
	thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(chassisKeyspace, "Members")
	if addErr != nil {
		err = fmt.Errorf("unable to add member to array for device: %s", xname)
		return
	}

	// Switches will use "EthernetSwitch_#" URL. This comes from the example redfish schema DSP-IS0004 v0.9c for ethernet switches.
	newInstanceKeyspace := chassisKeyspace + "/EthernetSwitch_0"
	helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", newInstanceKeyspace)

	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "@odata.type")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Name", switchInfo.Descr)
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Id", "EthernetSwitch_0")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "ChassisType", "Drawer")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Manufacturer", switchInfo.MfgName)
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "Model", switchInfo.ModelName)
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "SerialNumber", switchInfo.SerialNum)
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "AssetTag", switchInfo.AssetID)
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "PowerState", "On")
	helper.RedisHelper.setValueForPropertyInKeyspace(newInstanceKeyspace, "UUID", switchInfo.UUID)

	statusKeyspace, _ := helper.RedisHelper.addMemberToSet(newInstanceKeyspace, "Status")
	helper.RedisHelper.addMemberToSet(statusKeyspace, "State")
	helper.RedisHelper.setValueForPropertyInKeyspace(statusKeyspace, "Health", "OK")

	return
}

func (helper *SNMPSwitchHelper) initManagers(xname string, switchInfo snmp_utilities.EntityPhysicalTable) (err error) {
	// Initialize the Manager Collection
	err = helper.RedisHelper.initManagerCollection(xname)
	if err != nil {
		return fmt.Errorf("unable to init manager collection for %s: %w", xname, err)
	}

	// Initialize a Manager instance
	err = helper.RedisHelper.initManager(xname, "RTS", "SNMP Switch")
	if err != nil {
		return fmt.Errorf("unable to init manager for %s: %w", xname, err)
	}

	return
}

func (helper *SNMPSwitchHelper) initDevice(ctx context.Context, xname string, device slsapi.ManagementSwitch) (snmpDevice SNMPDevice, err error) {
	logFields := log.Fields{
		"xname": xname,
	}

	cred, credErr := compCredStore.GetCompCred(device.Xname)
	if credErr != nil {
		err = credErr
		logFields["err"] = err
		log.WithFields(logFields).Info("Unable to retrieve credentials")
		return
	}
	
	
	if len(cred.Username) < 1 {
		err = errors.New("zero length username in credentials, skipping initialization")
		logFields["err"] = err
		log.WithFields(logFields).Info("Bad vault credentials (username)")
		return
	}
	if len(cred.SNMPAuthPass) < 1 {
		err = errors.New("zero length SNMPAuthPass in credentials, skipping initialization")
		logFields["err"] = err
		log.WithFields(logFields).Info("Bad vault credentials (SNMPAuthPass)")
		return
	}
	if len(cred.SNMPPrivPass) < 1 {
		err = errors.New("zero length SNMPPrivPass in credentials, skipping initialization")
		logFields["err"] = err
		log.WithFields(logFields).Info("Bad vault credentials (SNMPPrivPass)")
		return
	}

	globCreds, credErr := helper.RTSCredentialStore.GetGlobalCredentials()
	if credErr != nil {
		err = credErr
		log.WithField("err", err).Info("Unable to get default credentails for RTS")
		return
	}
	// Use the global password if the entry doesn't already have a redfish password.
	cred.Password = globCreds.Password

	cred.URL = xname + "-rts:8083"

	// Use either the alias or IP address from SLS. Prefer alias because it is usually there.
	addr := ""
	if len(device.ExtraProperties.Aliases) < 1 {
		if device.ExtraProperties.IP4Addr == "" {
			log.WithFields(logFields).Info("Failed to create device. No address or alias in SLS")
			return
		} else {
			addr = device.ExtraProperties.IP4Addr
		}
	} else {
		addr = device.ExtraProperties.Aliases[0]
		// Need the Hardware Management Network address
		if !strings.HasSuffix(addr, ".hmn") {
			addr = addr + ".hmn"
		}
	}

	snmp, snmpErr := snmp_utilities.NewSNMPInterface(addr, cred.Username, cred.SNMPAuthPass, device.ExtraProperties.SNMPAuthProtocol, cred.SNMPPrivPass, device.ExtraProperties.SNMPPrivProtocol)
	if snmpErr != nil {
		err = snmpErr
		logFields["err"] = err
		log.WithFields(logFields).Info("Failed to create SNMP interface")
		return
	}

	snmpDevice.Xname = xname
	snmpDevice.SNMP = snmp

	switchInfo, snmpErr := snmp.GetPhysicalTable()
	if snmpErr != nil {
		err = snmpErr
		logFields["err"] = err
		log.WithFields(logFields).Info("Failed to get switch information")
		return
	}

	// First make sure there aren't any keys for this xname
	removeErr := helper.RedisHelper.removeXnamePrependedKeys(xname)
	if removeErr != nil {
		err = removeErr
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to remove keys from Redis")
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
	helper.RedisHelper.addMemberToSet(rootKeyspace, "Managers")

	// Create and protect a new active pipeline
	helper.RedisHelper.RedisActivePipelineMux.Lock()
	helper.RedisHelper.RedisActivePipeline = helper.RedisHelper.Redis.Pipeline()

	initFunctions := [...]func(string, snmp_utilities.EntityPhysicalTable) error{
		helper.initChassis,
		helper.initManagers,
	}

	for _, initFunction := range initFunctions {
		logFields["initFunction"] = runtime.FuncForPC(reflect.ValueOf(initFunction).Pointer()).Name()

		err = initFunction(xname, switchInfo)
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Error("Initialization function failed")

			helper.RedisHelper.RedisActivePipeline = nil
			helper.RedisHelper.RedisActivePipelineMux.Unlock()

			return
		} else {
			log.WithFields(logFields).Debug("Initialization function succeeded")
		}
	}
	delete(logFields, "initFunction")

	// Dump this pipeline.
	_, err = helper.RedisHelper.RedisActivePipeline.Exec()
	helper.RedisHelper.RedisActivePipeline = nil
	helper.RedisHelper.RedisActivePipelineMux.Unlock()
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to Exec() initInstance pipeline")
		return
	}

	role := "Administrator"
	// Setup Redfish Auth for this device.
	//TODO: Make redfish creds per device. For now, every device will answer via redfish with eachother's creds.
	err = helper.RedisHelper.initAccount(cred.Username, role, cred.Password)
	if err != nil {
		log.WithFields(log.Fields{
			"error":    err,
			"username": cred.Username,
			"role":     role,
		}).Error("problem encountered creating account")
		return
	}

	// Setup Certificate Service for the device
	err = helper.CertificateService.InitForXName(xname)
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Failed to initialize Certificate Service for device")
		return
	}

	log.WithFields(log.Fields{"device": xname}).Debug("Device initialized")

	// Create the DNS entry that will be used by HSM and friends to talk to this device. If this fails, no point in
	// continuing on or calling the rest of the process to this point a success as nobody will be able to talk to us.
	// The device will be getting the DNS name of x3000c0w42, so to differentiate RTS from the device will add the
	// `-rts` suffix
	err = informDNS(ctx, xname + "-rts")
	if err != nil {
		return
	}

	// Add the credentials to talk to RTS to Vault so HSM knows how to handle it.
	credErr = compCredStore.StoreCompCred(cred)
	if credErr != nil {
		err = credErr
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to store credentials in Vault")
		return
	}

	log.WithField("xname", xname).Debug("Added credentials to Vault")

	// Wait until the accountService has refreshed (happens every 30 seconds)
	for i := 0; i < 30; i++ {
		rdyErr := checkReady(ctx, xname)
		if rdyErr == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Now that all the necessary data is in Redis tell HSM we exist and have it discover us.
	hsmErr := informHSMWithFQDN(ctx, xname, cred.URL)
	if hsmErr != nil {
		err = hsmErr
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to inform HSM")
		return
	}

	log.WithField("xname", xname).Debug("Informed HSM")

	return
}

func (helper *SNMPSwitchHelper) populateRedisForSwitch(xname string, switchInfo snmp_utilities.EntityPhysicalTable) {
	pl := helper.RedisHelper.startPipeline()
	baseKeyspace := filepath.Join(xname, ChassisKeyspace, "EthernetSwitch_0")

	pl.Set(filepath.Join(baseKeyspace, "Name"), switchInfo.Descr, 0)

	pl.Set(filepath.Join(baseKeyspace, "Manufacturer"), switchInfo.MfgName, 0)
	pl.Set(filepath.Join(baseKeyspace, "Model"), switchInfo.ModelName, 0)
	pl.Set(filepath.Join(baseKeyspace, "SerialNumber"), switchInfo.SerialNum, 0)
	pl.Set(filepath.Join(baseKeyspace, "AssetTag"), switchInfo.AssetID, 0)
	pl.Set(filepath.Join(baseKeyspace, "UUID"), switchInfo.UUID, 0)

	pl.Exec()
	return
}

// If an interface has some refreshing it needs to do at some interval.
func (helper *SNMPSwitchHelper) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	// Get all the management switches in SLS.
	devices, err := helper.SLS.GetAllSwitchInfo()
	if err != nil {
		log.WithField("err", err).Error("Unable to get devices.")
		return
	}

	// Initialize devices for any management switches found in SLS.
	var deviceWaitGroup sync.WaitGroup
	start := time.Now()

	for _, device := range devices {
		xname := device.Xname
		// Check to see if we know this device exists already.
		if _, deviceIsKnown := helper.KnownDevices[xname]; deviceIsKnown {
			log.WithField("device", device).Trace("Skipping already known device")
			continue
		}

		log.WithField("device", device).Info("New device found, initiating discovery")

		deviceWaitGroup.Add(1)
		go func(xname string, device slsapi.ManagementSwitch) {
			defer deviceWaitGroup.Done()

			snmpDevice, initErr := helper.initDevice(ctx, xname, device)

			if initErr != nil {
				err = fmt.Errorf("unable to initialize device: %s", initErr)
			} else {
				helper.KnownDevicesMux.Lock()
				helper.KnownDevices[xname] = snmpDevice
				helper.KnownDevicesMux.Unlock()

				elapsed := time.Since(start)
				log.WithFields(log.Fields{
					"device":   xname,
					"initTime": elapsed,
				}).Debug("Done waiting for device to initialize")
			}
		}(xname, device)
	}
	deviceWaitGroup.Wait()

	return
}

// If an interface has some amount of prep work it needs to do before it is ready.
func (helper *SNMPSwitchHelper) SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error) {
	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport
	httpClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	slsURL, ok := os.LookupEnv("SLS_URL")
	if !ok {
		log.Fatal("Environment variable slsURL was not set")
	}

	serviceName, reqErr := base.GetServiceInstanceName()
	if reqErr != nil {
		serviceName = "RTS-SNMP"
		log.WithField("err", reqErr).Infof("WARNING, can't get service/instance name, using '%s'", serviceName)
	}
	helper.SLS = slsapi.NewSLS(slsURL, httpClient, serviceName)

	// Time to setup the connection to Vault
	rtsVaultKeypath, ok := os.LookupEnv("JAWS_VAULT_KEYPATH")
	if !ok {
		log.Fatal("Environment variable JAWS_VAULT_KEYPATH was not set")
	}

	secureStorage := connectToVault()
	if secureStorage == nil {
		log.Fatal("Unable to connect to Vault")
	}

	helper.RTSCredentialStore = rts_credential_store.NewRTSCredStore(rtsVaultKeypath, secureStorage)

	if ok := setupCompCredsVault(secureStorage); !ok {
		log.Fatal("Failed to setup CompCredsVault")
	}

	// Get default credentails from vault
	creds, credErr := helper.RTSCredentialStore.GetGlobalCredentials()
	if credErr != nil {
		err = credErr
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
		return
	}

	// Certificate Service setup
	err = helper.CertificateService.SetupCertificateService("RTS", GenericCertificateID)
	if err != nil {
		log.WithError(err).Error("Failed to setup certificate service")
		return
	}

	return
}

// Get Environment for backend handler invocations
func (helper *SNMPSwitchHelper) GetEnvForXname(xname string) (env map[string]string, err error) {
	env = map[string]string{}

	if snmpDevice, ok := helper.KnownDevices[xname]; ok {
		env["RTS_XNAME"] = snmpDevice.Xname

		return
	}

	err = errors.New("Unable to find xname in device list")
	log.WithField("xname", xname).Error(err)

	return
}

// Called for every request a backend might be able to service.
func (helper *SNMPSwitchHelper) RunBackendHelper(ctx context.Context, key string, args []string, env map[string]string) (value string, err error) {

	xname, ok := env["RTS_XNAME"]
	if !ok {
		err = errors.New("RTS_XNAME not in environment variables")
		log.Error(err.Error())
		return
	}

	// Check to make sure we actually know about this device.
	snmpDevice, deviceIsKnown := helper.KnownDevices[xname]
	if !deviceIsKnown {
		err = fmt.Errorf("unknown xname: %s", xname)
		log.Error(err.Error())
		return
	}

	// Do some sanity checks to make sure we're getting what we expect.
	// Every key that comes in could be xname prefixed, use a regular expression to capture that.
	keyRegex := regexp.MustCompile(`(x[0-9]{1,4}c[0-7]w[1-9][0-9]*)*(` + ServiceRootKeyspace + `\S+$)`)
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
		log.WithField("instancePrefixedKey", strippedKey).Error("Unable to derive the stripped key")
		return
	}

	// Here are all the actions that need to be supported. The basic formula they all have in common is to look at the
	// URL and determine what you need to do based on that. Since this is just a simulator all of the state information
	// is pushed directly into Redis so the next time it is requested that is the value that's given.
	if strippedKey == "/Chassis/EthernetSwitch_0/Status/State" {
		table, snmpErr := snmpDevice.SNMP.GetPhysicalTable()
		if snmpErr != nil {
			value = "UnavailableOffline"
			log.WithFields(log.Fields{"xname":xname, "error": snmpErr}).Info("Failed to retrieved switch PhysicalTable")
		} else {
			// Update values from the switch
			helper.populateRedisForSwitch(xname, table)
			value = "Enabled"
			log.Info("Retrieved switch PhysicalTable")
		}

		return
	}

	value, err = helper.RedisHelper.Redis.Get(key).Result()

	return
}