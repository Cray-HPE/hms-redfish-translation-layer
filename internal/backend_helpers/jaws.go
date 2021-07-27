// MIT License
//
// (C) Copyright [2019-2021] Hewlett Packard Enterprise Development LP
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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"

	compcredentials "github.com/Cray-HPE/hms-compcredentials"
	hmshttp "github.com/Cray-HPE/hms-go-http-lib"
	"github.com/go-redis/redis"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"

	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/pdu_credential_store"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfdispatcher/rts_credential_store"
)

const RackPDUsKeyspace = ServiceRootKeyspace + "/PowerEquipment/RackPDUs"

const EPOOutletGroupPrefix = "CrayEPO"

// PDU Structures

type Unit struct {
	Id                  string `mapstructure:"id"`
	Name                string `mapstructure:"name"`
	ManufactureDate     string `mapstructure:"manufacture_date"`
	ModelNumber         string `mapstructure:"model_number"`
	ProductSerialNumber string `mapstructure:"product_serial_number"`
	Type                string `mapstructure:"type"`

	Branches      []Branch
	Lines         []Line
	Phases        []Phase
	Outlets       []MonitorOutlet
	OutletConfigs []ConfigOutlet
	OutletGroups  []OutletGroup

	TemperatureSensors []TemperatureSensor
	HumiditySensors    []HumiditySensor
}

type MonitorUnit struct {
	Id                 string `mapstructure:"id"`
	Name               string `mapstructure:"name"`
	DisplayOrientation string `mapstructure:"display_orientation"`
	OutletSequence     string `mapstructure:"outlet_sequence"`
	Status             string `mapstructure:"status"`
	Type               string `mapstructure:"type"`
}

// Structures
type System struct {
	ActiveUsers           int    `json:"active_users"`
	Firmware              string `json:"firmware"`
	NICSerialNumber       string `json:"nic_serial_number"`
	StatusBranches        string `json:"status_branches"`
	StatusCords           string `json:"status_cords"`
	StatusLines           string `json:"status_lines"`
	StatusOCPs            string `json:"status_ocps"`
	StatusOutlets         string `json:"status_outlets"`
	StatusPhases          string `json:"status_phases"`
	StatusSensorsHumidity string `json:"status_sensors_humidity"`
	StatusSensorsTemp     string `json:"status_sensors_temp"`
	StatusUnits           string `json:"status_units"`
	Uptime                string `json:"uptime"`
}

type Branch struct {
	ID              string  `mapstructure:"id"`
	Name            string  `mapstructure:"name"`
	Current         float64 `mapstructure:"current"`
	CurrentCapacity float64 `mapstructure:"current_capacity"`
	CurrentStatus   string  `mapstructure:"current_status"`
	CurrentUtilized float64 `mapstructure:"current_utilized"`
	OCPID           string  `mapstructure:"ocp_id"`
	PhaseID         string  `mapstructure:"phase_id"`
	State           string  `mapstructure:"state"`
	Status          string  `mapstructure:"status"`
}

type Line struct {
	ID              string  `mapstructure:"id"`
	Name            string  `mapstructure:"name"`
	Current         float64 `mapstructure:"current"`
	CurrentCapacity int     `mapstructure:"current_capacity"`
	CurrentStatus   string  `mapstructure:"current_status"`
	CurrentUtilized float64 `mapstructure:"current_utilized"`
	State           string  `mapstructure:"state"`
	Status          string  `mapstructure:"status"`
}

type Phase struct {
	ID                string  `mapstructure:"id"`
	Name              string  `mapstructure:"name"`
	ActivePower       int     `mapstructure:"active_power"`
	ApparentPower     int     `mapstructure:"apparent_power"`
	CrestFactor       float64 `mapstructure:"crest_factor"`
	Current           float64 `mapstructure:"current"`
	Energy            int     `mapstructure:"energy"`
	NominalVoltage    int     `mapstructure:"nominal_voltage"`
	PowerFactor       float64 `mapstructure:"power_factor"`
	PowerFactorStatus string  `mapstructure:"power_factor_status"`
	Reactance         string  `mapstructure:"reactance"`
	State             string  `mapstructure:"state"`
	Status            string  `mapstructure:"status"`
	Voltage           float64 `mapstructure:"voltage"`
	VoltageStatus     string  `mapstructure:"voltage_status"`
	VoltageDeviation  float64 `mapstructure:"voltage_deviation"`
}

type MonitorOutlet struct {
	ID                string  `mapstructure:"id"`
	Name              string  `mapstructure:"name"`
	ActivePower       int     `mapstructure:"active_power"`
	ActivePowerStatus string  `mapstructure:"active_power_status"`
	ApparentPower     int     `mapstructure:"apparent_power"`
	BranchID          string  `mapstructure:"branch_id"`
	ControlState      string  `mapstructure:"control_state"`
	CrestFactor       float64 `mapstructure:"crest_factor"`
	Current           float64 `mapstructure:"current"`
	CurrentCapacity   int     `mapstructure:"current_capacity"`
	CurrentStatus     string  `mapstructure:"current_status"`
	CurrentUtilized   float64 `mapstructure:"current_utilized"`
	Energy            int     `mapstructure:"energy"`
	OCPID             string  `mapstructure:"ocp_id"`
	PhaseID           string  `mapstructure:"phase_id"`
	PowerCapacity     int     `mapstructure:"power_capacity"`
	PowerFactor       float64 `mapstructure:"power_factor"`
	PowerFactorStatus string  `mapstructure:"power_factor_status"`
	Reactance         string  `mapstructure:"reactance"`
	SocketAdapter     string  `mapstructure:"socket_adapter"`
	SocketType        string  `mapstructure:"socket_type"`
	State             string  `mapstructure:"state"`
	Status            string  `mapstructure:"status"`
	Voltage           float64 `mapstructure:"voltage"`
}

type ConfigOutlet struct {
	ID                    string    `mapstructure:"id"`
	Name                  string    `mapstructure:"name"`
	ControlLock           string    `mapstructure:"control_lock"`
	CurrentThresholds     []float64 `mapstructure:"current_thresholds"`
	EmailNotifications    string    `mapstructure:"email_notifications"`
	ExtraOnDelay          float64   `mapstructure:"extra_on_delay"`
	Host                  string    `mapstructure:"host"`
	PowerFactorThresholds []float64 `mapstructure:"power_factor_thresholds"`
	PowerThresholds       []float64 `mapstructure:"power_thresholds"`
	ScriptDelay           int       `mapstructure:"script_delay"`
	ScriptFeature         string    `mapstructure:"script_feature"`
	ShutdownDelay         int       `mapstructure:"shutdown_delay"`
	SNMPTrapNotifications string    `mapstructure:"snmp_trap_notifications"`
	WakeupState           string    `mapstructure:"wakeup_state"`
}

type OutletGroup struct {
	Name         string   `mapstructure:"name" json:"name"`
	OutletAccess []string `mapstructure:"outlet_access" json:"outlet_access"`
}

type TemperatureSensor struct {
	ID                 string   `mapstructure:"id" json:"id"`
	Name               string   `mapstructure:"name" json:"name"`
	Status             string   `mapstructure:"status" json:"status"`
	TemperatureCelsius *float64 `mapstructure:"temperature_celsius" json:"temperature_celsius"`
}

type HumiditySensor struct {
	ID               string   `mapstructure:"id" json:"id"`
	Name             string   `mapstructure:"name" json:"name"`
	Status           string   `mapstructure:"status" json:"status"`
	RelativeHumidity *float64 `mapstructure:"relative_humidity" json:"relative_humidity"`
}

/*
 * Redis Population
 */

// This function maps JAWS -> Health allowable values: "OK", "Warning", "Critical"
func translateJAWSASCIIToRedfishHealth(status string) string {
	status = strings.ToLower(status)
	if strings.Contains(status, "normal") || strings.Contains(status, "not found") {
		return "OK"
	} else if strings.Contains(status, "warning") {
		return "Warning"
	} else if strings.Contains(status, "alarm") {
		return "Critical"
	} else if strings.Contains(status, "not found") {
		return "Absent"
	} else {
		return "UNKNOWN"
	}
}

// This function maps JAWS -> Status State allowable values: "Absent", "Enabled"
func translateJAWSASCIIToRedfishState(status string) string {
	status = strings.ToLower(status)
	if strings.Contains(status, "not found") {
		return "Absent"
	}

	return "Enabled"
}

// This function maps JAWS -> Redfish Outlet naming convention
func translateJAWSToRedfishOutlet(outlet string) (string, error) {
	// PDU outlets exposed by Redfish need to have 0 padding to single digit outlet numbers.
	// This is needed for SMD to assign the correct PDU Power connector xnames to each outlet, as SMD sorts
	// the PDU outlet members lexicographically to determine the number of the outlet.

	// For example the following conversion takes place:
	// BA1  -> BA01
	// BA11 -> BA11

	if len(outlet) < 3 {
		return "", fmt.Errorf("outlet string is too short")
	}

	unitID := outlet[0:2]
	id, err := strconv.Atoi(outlet[2:])
	if err != nil {
		return "", fmt.Errorf("unable to parse outlet id: %w", err)
	}
	return fmt.Sprintf("%s%.2d", unitID, id), nil
}

// This function maps Redfish -> JAWS Outlet naming convention
func translateRedfishToJAWSOutlet(outlet string) (string, error) {
	// For example the following conversion takes place:
	// BA01 -> BA1
	// BA11 -> BA11

	if len(outlet) < 3 {
		return "", fmt.Errorf("outlet string is too short")
	}

	unitID := outlet[0:2]
	id, err := strconv.Atoi(outlet[2:])
	if err != nil {
		return "", fmt.Errorf("unable to parse outlet id: %w", err)
	}
	return fmt.Sprintf("%s%d", unitID, id), nil
}

// Structure specific functions

func (helper JAWSBackedHelper) populateRedisForMonitorUnits(pl redis.Pipeliner, units []MonitorUnit, xname string,
	pduID string) (err error) {
	timeout := time.Minute * 1
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Status")

	for _, unit := range units {
		if unit.Id == pduID {
			pl.Set(filepath.Join(baseKeyspace, "Health"),
				translateJAWSASCIIToRedfishHealth(unit.Status), timeout)
			return
		}
	}

	return
}

func (helper JAWSBackedHelper) populateRedisForBranch(pl redis.Pipeliner, branch Branch, xname string, pduID string) (err error) {
	timeout := time.Second * 15
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Branches", branch.ID)

	pl.Set(filepath.Join(baseKeyspace, "PowerState"), branch.State, timeout)

	// CurrentSensor
	pl.Set(filepath.Join(baseKeyspace, "CurrentSensor", "Reading"), branch.Current, timeout)
	pl.Set(filepath.Join(baseKeyspace, "CurrentSensor", "Status", "Health"),
		translateJAWSASCIIToRedfishHealth(branch.CurrentStatus), timeout)

	pl.Set(filepath.Join(baseKeyspace, "Status", "Health"),
		translateJAWSASCIIToRedfishHealth(branch.Status), timeout)

	return
}

func (helper JAWSBackedHelper) populateRedisForLines(pl redis.Pipeliner, lines []Line, xname string, pduID string) (err error) {
	timeout := time.Second * 30
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Mains", "AC1", "PolyPhaseCurrentSensors")

	for _, line := range lines {
		if !strings.HasPrefix(line.ID, pduID) {
			log.WithFields(log.Fields{
				"pduID":   pduID,
				"line.ID": line.ID,
			}).Debug("Ignoring Phase for this PDU")
			continue
		}

		lineIndex := getIndexFromJAWsID(line.ID)
		lineKeyspace := filepath.Join(baseKeyspace, "Line"+lineIndex)

		pl.Set(filepath.Join(lineKeyspace, "Reading"), line.Current, timeout)
		pl.Set(filepath.Join(lineKeyspace, "Status", "Health"),
			translateJAWSASCIIToRedfishHealth(line.CurrentStatus), timeout)
	}

	return
}

func (helper JAWSBackedHelper) populateRedisForPhases(pl redis.Pipeliner, phases []Phase, xname string, pduID string) (err error) {
	timeout := time.Second * 30
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Mains", "AC1", "PolyPhasePowerSensors")

	for _, phase := range phases {
		if !strings.HasPrefix(phase.ID, pduID) {
			log.WithFields(log.Fields{
				"pduID":    pduID,
				"phase.ID": phase.ID,
			}).Debug("Ignoring Phase for this PDU")
			continue
		}

		fromPhase, toPhase := getFromAndToPhasesFromPhaseName(phase.Name)
		phaseKeyspace := filepath.Join(baseKeyspace, "Line"+fromPhase+"ToLine"+toPhase)

		pl.Set(filepath.Join(phaseKeyspace, "ApparentVA"), phase.ApparentPower, timeout)
		pl.Set(filepath.Join(phaseKeyspace, "PowerFactor"), phase.PowerFactor, timeout)
		pl.Set(filepath.Join(phaseKeyspace, "Reading"), phase.ActivePower, timeout)
	}

	return
}

func (helper JAWSBackedHelper) populateRedisForMonitorOutlet(pl redis.Pipeliner, outlet MonitorOutlet, xname string,
	pduID string) (err error) {
	timeout := time.Second * 10
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	outletID, err := translateJAWSToRedfishOutlet(outlet.ID)
	if err != nil {
		return err
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Outlets", outletID)

	pl.Set(filepath.Join(baseKeyspace, "PowerState"), outlet.State, timeout)
	pl.Set(filepath.Join(baseKeyspace, "RatedCurrentAmps"), outlet.CurrentCapacity, timeout)

	// CurrentSensor
	pl.Set(filepath.Join(baseKeyspace, "CurrentSensor", "Reading"), outlet.Current, timeout)
	pl.Set(filepath.Join(baseKeyspace, "CurrentSensor", "Status", "Health"),
		translateJAWSASCIIToRedfishHealth(outlet.CurrentStatus), timeout)

	// EnergySensor
	pl.Set(filepath.Join(baseKeyspace, "EnergySensor", "Reading"), outlet.Energy, timeout)

	// PowerSensor
	pl.Set(filepath.Join(baseKeyspace, "PowerSensor", "ApparentVA"), outlet.ApparentPower, timeout)
	pl.Set(filepath.Join(baseKeyspace, "PowerSensor", "PowerFactor"), outlet.PowerFactor, timeout)
	pl.Set(filepath.Join(baseKeyspace, "PowerSensor", "Reading"), outlet.ActivePower, timeout)
	pl.Set(filepath.Join(baseKeyspace, "PowerSensor", "Status", "Health"),
		translateJAWSASCIIToRedfishHealth(outlet.ActivePowerStatus), timeout)

	// VoltageSensor
	pl.Set(filepath.Join(baseKeyspace, "VoltageSensor", "Reading"), outlet.Voltage, timeout)

	// Status
	pl.Set(filepath.Join(baseKeyspace, "Status", "Health"),
		translateJAWSASCIIToRedfishHealth(outlet.Status), timeout)

	return
}

func (helper JAWSBackedHelper) populateRedisForConfigOutlet(pl redis.Pipeliner, outlet ConfigOutlet, xname string,
	pduID string) (err error) {
	timeout := time.Second * 10
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	outletID, err := translateJAWSToRedfishOutlet(outlet.ID)
	if err != nil {
		return err
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Outlets", outletID)

	// Have to map JAWS states (last | off | on) -> Redfish states ("AlwaysOn", "AlwaysOff", "LastState")
	var powerRestorePolicy string
	if outlet.WakeupState == "on" {
		powerRestorePolicy = "AlwaysOn"
	} else if outlet.WakeupState == "off" {
		powerRestorePolicy = "AlwaysOff"
	} else if outlet.WakeupState == "last" {
		powerRestorePolicy = "LastState"
	} else {
		powerRestorePolicy = "UNKNOWN"
	}
	pl.Set(filepath.Join(baseKeyspace, "PowerRestorePolicy"), powerRestorePolicy, timeout)

	return
}

func (helper JAWSBackedHelper) populateRedisForTemperature(pl redis.Pipeliner, sensors []TemperatureSensor, xname string, pduID string) (err error) {
	timeout := time.Second * 30
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Sensors")

	for _, sensor := range sensors {
		sensorID := "Temperature" + sensor.ID
		sensorKeyspace := filepath.Join(baseKeyspace, sensorID)

		// Note: If the PDU does not have a temperature sensor, the field TemperatureCelsius will be nil.
		// To represent a null number in redis it will have the value of an empty string
		var sensorReading interface{}
		if sensor.TemperatureCelsius != nil {
			sensorReading = *sensor.TemperatureCelsius
		} else {
			sensorReading = ""
		}

		pl.Set(filepath.Join(sensorKeyspace, "Reading"), sensorReading, timeout)
		pl.Set(filepath.Join(sensorKeyspace, "Status", "Health"),
			translateJAWSASCIIToRedfishHealth(sensor.Status), timeout)
		pl.Set(filepath.Join(sensorKeyspace, "Status", "State"),
			translateJAWSASCIIToRedfishState(sensor.Status), timeout)
	}

	return
}

func (helper JAWSBackedHelper) populateRedisForHumidity(pl redis.Pipeliner, sensors []HumiditySensor, xname string, pduID string) (err error) {
	timeout := time.Second * 30
	if helper.PollingEnabled {
		// Disable key expiration if we are polling the PDU. This will prevent unnecessary requests to made to the PDU
		timeout = 0
	}

	baseKeyspace := filepath.Join(xname+RackPDUsKeyspace, pduID, "Sensors")

	for _, sensor := range sensors {
		sensorID := "Humidity" + sensor.ID
		sensorKeyspace := filepath.Join(baseKeyspace, sensorID)

		// Note: If the PDU does not have a Humidity sensor, the field RelativeHumidity will be nil.
		// To represent a null number in redis it will have the value of an empty string
		var sensorReading interface{}
		if sensor.RelativeHumidity != nil {
			sensorReading = *sensor.RelativeHumidity
		} else {
			sensorReading = ""
		}
		pl.Set(filepath.Join(sensorKeyspace, "Reading"), sensorReading, timeout)
		pl.Set(filepath.Join(sensorKeyspace, "Status", "Health"),
			translateJAWSASCIIToRedfishHealth(sensor.Status), timeout)
		pl.Set(filepath.Join(sensorKeyspace, "Status", "State"),
			translateJAWSASCIIToRedfishState(sensor.Status), timeout)
	}

	return
}

/*
 * Setup
 */

func getIndexFromJAWsID(lineID string) string {
	lineIndexRegex := regexp.MustCompile(`[A-Z]+(\d+)`)
	indexMatches := lineIndexRegex.FindStringSubmatch(lineID)
	if indexMatches != nil {
		return indexMatches[1]
	}

	return "INVALID"
}

func getFromAndToPhasesFromPhaseName(phaseName string) (string, string) {
	phaseRegex := regexp.MustCompile(`[A-Z]+:L(\d+)-L(\d+)`)
	phases := phaseRegex.FindStringSubmatch(phaseName)
	if phases == nil {
		return "INVALID", "INVALID"
	}

	return phases[1], phases[2]
}

func (helper JAWSBackedHelper) removeXnamePrependedKeys(ctx context.Context, xname string) (err error) {
	logFields := log.Fields{
		"xname": xname,
	}

	xnameKeys, err := helper.RedisHelper.Redis.Keys(xname + "*").Result()
	if err != nil {
		logFields["err"] = err
		log.WithFields(logFields).Error("Unable to get keys for xname")
		return
	}

	pipe := helper.RedisHelper.Redis.Pipeline()

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
func (helper JAWSBackedHelper) invalidateRedisKeys(keys []string) {
	logFields := log.Fields{
		"keys": keys,
	}

	for _, key := range keys {
		err := helper.RedisHelper.Redis.Del(key).Err()
		if err == nil {
			log.WithFields(logFields).Debug("Removed key")
		} else {
			logFields["err"] = err
			log.WithFields(logFields).Error("Unable to remove key")
		}
	}
}

func (helper JAWSBackedHelper) initRackPDUBranches(xname string, unit Unit) (err error) {
	keyspace, err := helper.RedisHelper.addMemberToSet(filepath.Join(xname, RackPDUsKeyspace, unit.Id), "Branches")

	helper.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	for _, branch := range unit.Branches {
		thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(keyspace, "Members")
		if addErr != nil {
			err = fmt.Errorf("unable to add outlet member to array for PDU %s", unit.Id)
			return
		}

		thisKeyspace := filepath.Join(keyspace, branch.ID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", thisKeyspace)

		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.type")

		// Generic
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Id", branch.ID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Name", branch.Name)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "CircuitType", "Branch")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "PowerState")

		// CurrentSensor
		currentSensorKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "CurrentSensor")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for Branch")
			return
		}
		helper.RedisHelper.setValueForPropertyInKeyspace(currentSensorKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "CurrentBranch"+branch.ID))
		helper.RedisHelper.setValueForPropertyInKeyspace(currentSensorKeyspace, "Name",
			"Branch "+branch.ID+" Current")
		helper.RedisHelper.addMemberToSet(currentSensorKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(currentSensorKeyspace, "ReadingUnits", "A")

		currentSensorStatusKeyspace, currentAddErr := helper.RedisHelper.addMemberToSet(currentSensorKeyspace, "Status")
		if currentAddErr != nil {
			err = fmt.Errorf("unable to add member to array for CurrentSensor")
			return
		}
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "Health")

		// Status
		statusKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "Status")
		if addErr != nil {
			err = errors.New("unable to add member to array")
			return
		}
		helper.RedisHelper.addMemberToSet(statusKeyspace, "Health")
	}

	return
}

func (helper JAWSBackedHelper) initRackPDUMains(xname string, unit Unit) (err error) {
	keyspace, err := helper.RedisHelper.addMemberToSet(filepath.Join(xname, RackPDUsKeyspace, unit.Id), "Mains")

	helper.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	// Mains are a bit different than most as we don't loop over every instance, they all belong to a single main input.
	thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(keyspace, "Members")
	if addErr != nil {
		err = fmt.Errorf("unable to add outlet member to array for PDU %s", unit.Id)
		return
	}

	// Hardcode the keyspace as AC1.
	thisKeyspace := filepath.Join(keyspace, "AC1")
	helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", thisKeyspace)

	helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.type")

	// Generic
	helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Id", "AC1")
	helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Name", "Mains AC Input")
	helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "CircuitType", "Mains")
	helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "NominalVoltage",
		"AC"+strconv.Itoa(unit.Phases[0].NominalVoltage)+"V")

	// PolyPhaseCurrentSensors
	polyPhaseCurrentSensorsKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "PolyPhaseCurrentSensors")
	if addErr != nil {
		err = fmt.Errorf("unable to add member to array for Mains")
		return
	}
	// Each line corresponds to its own keyspace.
	for _, line := range unit.Lines {
		// This could be out of order...need to actually deduce the correct index.
		lineIndex := getIndexFromJAWsID(line.ID)

		lineKeyspace, addErr := helper.RedisHelper.addMemberToSet(polyPhaseCurrentSensorsKeyspace, "Line"+lineIndex)
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for PolyPhaseCurrentSensors")
			return
		}
		helper.RedisHelper.setValueForPropertyInKeyspace(lineKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "CurrentMains1-"+line.ID))
		helper.RedisHelper.setValueForPropertyInKeyspace(lineKeyspace, "Name",
			"Mains Current L"+lineIndex)
		helper.RedisHelper.addMemberToSet(lineKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(lineKeyspace, "ReadingUnits", "A")

		currentSensorStatusKeyspace, currentAddErr := helper.RedisHelper.addMemberToSet(lineKeyspace, "Status")
		if currentAddErr != nil {
			err = fmt.Errorf("unable to add member to array for PolyPhaseCurrentSensor")
			return
		}
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "Health")
	}

	// PolyPhasePowerSensors
	polyPhasePowerSensorsKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "PolyPhasePowerSensors")
	if addErr != nil {
		err = fmt.Errorf("unable to add member to array for Mains")
		return
	}
	// Each phase corresponds to its own keyspace.
	for _, phase := range unit.Phases {
		// Phases could be out of order, the only way to know what phase we're talking about is to deduce it.
		fromPhase, toPhase := getFromAndToPhasesFromPhaseName(phase.Name)

		phaseKeyspace, addErr := helper.RedisHelper.addMemberToSet(polyPhasePowerSensorsKeyspace,
			"Line"+fromPhase+"ToLine"+toPhase)
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for polyPhasePowerSensorsKeyspace")
			return
		}
		helper.RedisHelper.addMemberToSet(phaseKeyspace, "ApparentVA")
		helper.RedisHelper.setValueForPropertyInKeyspace(phaseKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "PowerMains1-"+phase.ID))
		helper.RedisHelper.setValueForPropertyInKeyspace(phaseKeyspace, "Name",
			"Mains Power L"+fromPhase+"L"+toPhase)
		helper.RedisHelper.addMemberToSet(phaseKeyspace, "PowerFactor")
		helper.RedisHelper.addMemberToSet(phaseKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(phaseKeyspace, "ReadingUnits", "W")
	}

	return
}

func (helper JAWSBackedHelper) initRackPDUOutlets(xname string, unit Unit) (err error) {
	keyspace, err := helper.RedisHelper.addMemberToSet(filepath.Join(xname, RackPDUsKeyspace, unit.Id), "Outlets")

	helper.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	for _, outlet := range unit.Outlets {
		thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(keyspace, "Members")
		if addErr != nil {
			err = fmt.Errorf("unable to add outlet member to array for PDU %s", unit.Id)
			return
		}

		var outletID string
		outletID, err = translateJAWSToRedfishOutlet(outlet.ID)
		if err != nil {
			return err
		}

		thisKeyspace := filepath.Join(keyspace, outletID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", thisKeyspace)

		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.type")

		// Generic
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Id", outletID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Name", outlet.Name)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "OutletType", outlet.SocketType)
		helper.RedisHelper.addMemberToSet(thisKeyspace, "PowerRestorePolicy")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "PowerState")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "RatedCurrentAmps")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "VoltageType", "AC")

		// Actions
		actionsKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "Actions")
		if addErr != nil {
			err = fmt.Errorf("unable to add Actions to outlet %s", outlet.Name)
			return
		}
		actionName := "Outlet.PowerControl"
		powerControlKeyspace, addErr := helper.RedisHelper.addMemberToSet(actionsKeyspace, "#"+actionName)
		powerControlTargetURI := filepath.Join(actionsKeyspace, actionName)
		helper.RedisHelper.setValueForPropertyInKeyspace(powerControlKeyspace, "Target", powerControlTargetURI)

		allowableValuesKeyspace, addErr := helper.RedisHelper.addMemberToSet(powerControlKeyspace,
			"PowerState@Redfish.AllowableValues")
		if addErr != nil {
			err = fmt.Errorf("unable to add AllowableValues to PowerControl for outlet %s", outlet.Name)
			return
		}
		helper.RedisHelper.Redis.RPush(allowableValuesKeyspace, "On", "Off")

		// CurrentSensor
		currentSensorKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "CurrentSensor")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for Outlet")
			return
		}
		helper.RedisHelper.setValueForPropertyInKeyspace(currentSensorKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "CurrentOutlet"+outletID))
		helper.RedisHelper.setValueForPropertyInKeyspace(currentSensorKeyspace, "Name",
			"Outlet "+outlet.ID+" Current")
		helper.RedisHelper.addMemberToSet(currentSensorKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(currentSensorKeyspace, "ReadingUnits", "A")

		currentSensorStatusKeyspace, currentAddErr := helper.RedisHelper.addMemberToSet(currentSensorKeyspace, "Status")
		if currentAddErr != nil {
			err = fmt.Errorf("unable to add member to array for CurrentSensor")
			return
		}
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "Health")

		// EnergySensor
		energySensorKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "EnergySensor")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for Outlet")
			return
		}
		helper.RedisHelper.setValueForPropertyInKeyspace(energySensorKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "Energy"+outletID))
		helper.RedisHelper.setValueForPropertyInKeyspace(energySensorKeyspace, "Name",
			"Outlet "+outletID+" Energy")
		helper.RedisHelper.addMemberToSet(energySensorKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(energySensorKeyspace, "ReadingUnits", "J")

		// PowerSensor
		powerSensorKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "PowerSensor")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for Outlet")
			return
		}
		helper.RedisHelper.addMemberToSet(powerSensorKeyspace, "ApparentVA")
		helper.RedisHelper.setValueForPropertyInKeyspace(powerSensorKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "Power"+outletID))
		helper.RedisHelper.setValueForPropertyInKeyspace(powerSensorKeyspace, "Name",
			"Outlet "+outletID+" Power")
		helper.RedisHelper.addMemberToSet(powerSensorKeyspace, "PowerFactor")
		helper.RedisHelper.addMemberToSet(powerSensorKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(powerSensorKeyspace, "ReadingUnits", "W")

		powerSensorStatusKeyspace, currentAddErr := helper.RedisHelper.addMemberToSet(powerSensorKeyspace, "Status")
		if currentAddErr != nil {
			err = fmt.Errorf("unable to add member to array for PowerSensor")
			return
		}
		helper.RedisHelper.addMemberToSet(powerSensorStatusKeyspace, "Health")

		// Status
		statusKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "Status")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for Outlet")
			return
		}
		helper.RedisHelper.addMemberToSet(statusKeyspace, "Health")

		// VoltageSensor
		voltageSensorKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "VoltageSensor")
		if addErr != nil {
			err = fmt.Errorf("unable to add member to array for Outlet")
			return
		}
		helper.RedisHelper.setValueForPropertyInKeyspace(voltageSensorKeyspace, "DataSourceUri",
			filepath.Join(RackPDUsKeyspace, unit.Id, "Sensors", "Voltage"+outletID))
		helper.RedisHelper.setValueForPropertyInKeyspace(voltageSensorKeyspace, "Name",
			"Outlet "+outletID+" Voltage")
		helper.RedisHelper.addMemberToSet(voltageSensorKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(voltageSensorKeyspace, "ReadingUnits", "V")
	}

	return
}

func (helper JAWSBackedHelper) initRackPDUOutletGroups(xname string, unit Unit) (err error) {
	keyspace, err := helper.RedisHelper.addMemberToSet(filepath.Join(xname, RackPDUsKeyspace, unit.Id), "OutletGroups")

	helper.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(keyspace, "@odata.type")

	for _, outletGroup := range unit.OutletGroups {
		thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(keyspace, "Members")
		if addErr != nil {
			err = fmt.Errorf("unable to add outlet group member to array for PDU %s", unit.Id)
			return
		}

		thisKeyspace := filepath.Join(keyspace, outletGroup.Name)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", thisKeyspace)

		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.type")

		// Generic
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Name", outletGroup.Name)

		// Actions
		actionsKeyspace, addErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "Actions")
		if addErr != nil {
			err = fmt.Errorf("unable to add Actions to outlet group %s", outletGroup.Name)
			return
		}
		actionName := "OutletGroup.PowerControl"
		powerControlKeyspace, addErr := helper.RedisHelper.addMemberToSet(actionsKeyspace, "#"+actionName)
		powerControlTargetURI := filepath.Join(actionsKeyspace, actionName)
		helper.RedisHelper.setValueForPropertyInKeyspace(powerControlKeyspace, "Target", powerControlTargetURI)

		allowableValuesKeyspace, addErr := helper.RedisHelper.addMemberToSet(powerControlKeyspace,
			"PowerState@Redfish.AllowableValues")
		if addErr != nil {
			err = fmt.Errorf("unable to add AllowableValues to PowerControl for outlet group %s",
				outletGroup.Name)
			return
		}
		helper.RedisHelper.Redis.RPush(allowableValuesKeyspace, "On", "Off")

		// Outlets
		for _, outlet := range outletGroup.OutletAccess {
			thisOutletKeyspace, addErr := helper.RedisHelper.addMemberToArray(thisKeyspace, "Outlets")
			if addErr != nil {
				err = fmt.Errorf("unable to add outlets array for outlet group %s on PDU %s", outletGroup.Name,
					unit.Id)
				return
			}
			helper.RedisHelper.setValueForPropertyInKeyspace(thisOutletKeyspace, "@odata.id",
				filepath.Join(RackPDUsKeyspace, unit.Id, "Outlets", outlet))
		}
	}

	return
}

func (helper JAWSBackedHelper) initRackPDUSensors(xname string, unit Unit) (err error) {
	// keyspace, err := helper.RedisHelper.addMemberToSet(filepath.Join(xname, RackPDUsKeyspace, unit.Id), "Branches")
	keyspace, err := helper.RedisHelper.addMemberToSet(filepath.Join(xname, RackPDUsKeyspace, unit.Id), "Sensors")

	// Setup sensor collection
	helper.RedisHelper.addMemberToSet(keyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(keyspace, "@odata.type")
	helper.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Name", "PDU Sensor Collection")
	helper.RedisHelper.setValueForPropertyInKeyspace(keyspace, "Description", "A collection of sensor attached to this PDU instance")

	// Temperature Sensors
	for _, sensor := range unit.TemperatureSensors {
		thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(keyspace, "Members")
		if addErr != nil {
			err = fmt.Errorf("unable to add temperature sensor member to array for PDU %s", unit.Id)
			return
		}

		sensorID := "Temperature" + sensor.ID

		thisKeyspace := filepath.Join(keyspace, sensorID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", thisKeyspace)

		// Generic Fields
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.type")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Id", sensorID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Name", sensor.Name)

		// Temperature fields
		helper.RedisHelper.addMemberToSet(thisKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "ReadingType", "Temperature")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "ReadingUnits", "C")

		// Status field
		currentSensorStatusKeyspace, currentAddErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "Status")
		if currentAddErr != nil {
			err = fmt.Errorf("unable to add member to array for Temperature Sensor")
			return
		}
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "Health")
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "State")
	}

	// Humidity
	for _, sensor := range unit.HumiditySensors {
		thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(keyspace, "Members")
		if addErr != nil {
			err = fmt.Errorf("unable to add humidity sensor member to array for PDU %s", unit.Id)
			return
		}

		sensorID := "Humidity" + sensor.ID

		thisKeyspace := filepath.Join(keyspace, sensorID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", thisKeyspace)

		// Generic Fields
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.id")
		helper.RedisHelper.addMemberToSet(thisKeyspace, "@odata.type")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Id", sensorID)
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "Name", sensor.Name)

		// Temperature fields
		helper.RedisHelper.addMemberToSet(thisKeyspace, "Reading")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "ReadingType", "Humidity")
		helper.RedisHelper.setValueForPropertyInKeyspace(thisKeyspace, "ReadingUnits", "%")

		// Status field
		currentSensorStatusKeyspace, currentAddErr := helper.RedisHelper.addMemberToSet(thisKeyspace, "Status")
		if currentAddErr != nil {
			err = fmt.Errorf("unable to add member to array for Humidity Sensor")
			return
		}
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "Health")
		helper.RedisHelper.addMemberToSet(currentSensorStatusKeyspace, "State")
	}

	return
}

func (helper JAWSBackedHelper) initRackPDU(xname string, unit Unit) (err error) {
	logFields := log.Fields{
		"xname":   xname,
		"unit.Id": unit.Id,
	}

	// Have to start from the very beginning, it's the only way we know if a PDU actually exists.
	rootKeyspace, _ := helper.RedisHelper.addMemberToSet("/redfish", "v1")
	helper.RedisHelper.setValueForPropertyInKeyspace(rootKeyspace, "Name", "Service Root")
	helper.RedisHelper.setValueForPropertyInKeyspace(rootKeyspace, "Description", "The Redfish ServiceRoot")
	helper.RedisHelper.addMemberToSet(rootKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(rootKeyspace, "@odata.type")

	versionStr, ok := os.LookupEnv("SCHEMA_VERSION")
	if ok {
		helper.RedisHelper.setValueForPropertyInKeyspace(rootKeyspace, "RedfishVersion", versionStr)
	}

	powerEquipmentKeyspace, _ := helper.RedisHelper.addMemberToSet(rootKeyspace, "PowerEquipment")
	helper.RedisHelper.setValueForPropertyInKeyspace(powerEquipmentKeyspace, "Name", "Service Root")
	helper.RedisHelper.addMemberToSet(powerEquipmentKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(powerEquipmentKeyspace, "@odata.type")

	helper.RedisHelper.addMemberToSet(powerEquipmentKeyspace, "RackPDUs")

	// The keyspace for this PDU
	xnameKeyspace := xname + RackPDUsKeyspace

	// Create a new pipeline
	helper.RedisHelper.RedisActivePipeline = helper.RedisHelper.Redis.Pipeline()

	helper.RedisHelper.setValueForPropertyInKeyspace(xnameKeyspace, "Name", "RackPDU Collection")

	helper.RedisHelper.addMemberToSet(xnameKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(xnameKeyspace, "@odata.type")

	// Add this PDU to the Members of RackPDUs for this xname.
	thisMemberKeyspace, addErr := helper.RedisHelper.addMemberToArray(xnameKeyspace, "Members")
	if addErr != nil {
		err = fmt.Errorf("unable to add member to array for PDU %s", unit.Id)
		return
	}

	newPDUKeyspace := xnameKeyspace + "/" + unit.Id
	helper.RedisHelper.setValueForPropertyInKeyspace(thisMemberKeyspace, "@odata.id", newPDUKeyspace)

	helper.RedisHelper.addMemberToSet(newPDUKeyspace, "@odata.id")
	helper.RedisHelper.addMemberToSet(newPDUKeyspace, "@odata.type")

	// Basic members always necessary.
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "DateOfManufacture", unit.ManufactureDate)
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "Name", unit.Name)
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "Id", unit.Id)
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "Manufacturer", "ServerTech")
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "Model", unit.ModelNumber)
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "SerialNumber", unit.ProductSerialNumber)
	helper.RedisHelper.setValueForPropertyInKeyspace(newPDUKeyspace, "EquipmentType", "RackPDU")

	statusKeyspace, err := helper.RedisHelper.addMemberToSet(newPDUKeyspace, "Status")
	helper.RedisHelper.addMemberToSet(statusKeyspace, "Health")

	circuitSummaryKeyspace, err := helper.RedisHelper.addMemberToSet(newPDUKeyspace, "CircuitSummary")
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "ControlledOutlets",
		strconv.Itoa(len(unit.Outlets)))
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "MonitoredBranches",
		strconv.Itoa(len(unit.Branches)))
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "MonitoredOutlets",
		strconv.Itoa(len(unit.Outlets)))
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "MonitoredPhases",
		strconv.Itoa(len(unit.Phases)))
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "TotalBranches",
		strconv.Itoa(len(unit.Branches)))
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "TotalOutlets",
		strconv.Itoa(len(unit.Outlets)))
	helper.RedisHelper.setValueForPropertyInKeyspace(circuitSummaryKeyspace, "TotalPhases",
		strconv.Itoa(len(unit.Phases)))

	initFunctions := [...]func(string, Unit) error{
		helper.initRackPDUBranches,
		helper.initRackPDUMains,
		helper.initRackPDUOutlets,
		helper.initRackPDUOutletGroups,
		helper.initRackPDUSensors,
	}

	for _, initFunction := range initFunctions {
		logFields["initFunction"] = runtime.FuncForPC(reflect.ValueOf(initFunction).Pointer()).Name()

		err = initFunction(xname, unit)
		if err != nil {
			logFields["err"] = err
			log.WithFields(logFields).Error("Initialization function failed")

			return
		} else {
			log.WithFields(logFields).Debug("Initialization function succeeded")
		}
	}

	// Since we already have polled the PDU for information, might as well update redis with it
	helper.populateRedisForLines(helper.RedisHelper.RedisActivePipeline, unit.Lines, xname, unit.Id)
	helper.populateRedisForPhases(helper.RedisHelper.RedisActivePipeline, unit.Phases, xname, unit.Id)
	helper.populateRedisForTemperature(helper.RedisHelper.RedisActivePipeline, unit.TemperatureSensors, xname, unit.Id)
	helper.populateRedisForHumidity(helper.RedisHelper.RedisActivePipeline, unit.HumiditySensors, xname, unit.Id)
	for _, branch := range unit.Branches {
		helper.populateRedisForBranch(helper.RedisHelper.RedisActivePipeline, branch, xname, unit.Id)
	}
	for _, outlet := range unit.OutletConfigs {
		helper.populateRedisForConfigOutlet(helper.RedisHelper.RedisActivePipeline, outlet, xname, unit.Id)
	}
	for _, outlet := range unit.Outlets {
		helper.populateRedisForMonitorOutlet(helper.RedisHelper.RedisActivePipeline, outlet, xname, unit.Id)
	}

	// Dump this pipeline.
	_, err = helper.RedisHelper.RedisActivePipeline.Exec()
	helper.RedisHelper.RedisActivePipeline = nil
	if err != nil {
		log.WithFields(logFields).Error("Unable to Exec() initRackPDU pipeline")
		return
	}

	log.WithFields(logFields).Info("Unit successfully initialized")

	return
}

func (helper JAWSBackedHelper) initDevice(ctx context.Context, xname string,
	device pdu_credential_store.Device) (err error) {
	var v interface{}
	var getErr error

	logCtx := log.WithField("xname", device.Xname)

	auth := hmshttp.Auth{
		Username: device.Username,
		Password: device.Password,
	}
	baseRequest := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		ExpectedStatusCodes: []int{http.StatusOK},
		Method:              "GET",
		CustomHeaders:       getSvcInstName(),
		Auth:                &auth,
	}

	// Disable DHCP static address fallback address
	logCtx.Info("Disabling DHCP Static address fallback")
	err = helper.disableDHCPStaticAddressFallback(baseRequest, device)
	if err != err {
		logCtx.WithField("err", err).Error("Unable to disable DHCP static address fallback option on PDU")
		// This is a nice to have but not a fatal condition, so we can solider on with the rest of the PDU initialization
	}

	// First, get all the units for this master.
	unitsRequest := baseRequest
	unitsRequest.FullURL = device.URL + "/config/info/units"
	v, getErr = unitsRequest.GetBodyForHTTPRequest()
	if v == nil {
		err = fmt.Errorf("unable to get body for units request: %s", getErr)
		return
	}

	unitsInterface := v.([]interface{})
	var units []Unit
	err = mapstructure.Decode(unitsInterface, &units)
	if err != nil {
		logCtx.WithField("unitsInterface", unitsInterface).Error("Unable to decode unitsInterface")
		return
	}

	// Since JAWS exposes everything under universal URLs the only way to tell which unit something belongs to is
	// to look at them all and use their names to manually build up the structures. First gather all of the
	// necessary interfaces and then in one big loop assign them to the right unit.

	// Branches
	branches, err := helper.getMonitorBranches(baseRequest, device)
	if err != nil {
		logCtx.WithField("err", err).Error("Unable to get monitor branches")
		return
	}

	/*
	 * Mains
	 */
	// Lines
	lines, err := helper.getMonitorLines(baseRequest, device)
	if err != nil {
		logCtx.WithField("err", err).Error("Unable to get monitor lines")
		return
	}

	// Phases
	phases, err := helper.getMonitorPhases(baseRequest, device)
	if err != nil {
		log.WithField("err", err).Error("Unable to get monitor phases")
		return
	}

	// Outlets
	outlets, err := helper.getMonitorOutlets(baseRequest, device)
	if err != nil {
		logCtx.WithField("err", err).Error("Unable to get monitor outlets")
		return
	}

	// Outlet configs
	outletConfigs, err := helper.getConfigOutlets(baseRequest, device)
	if err != nil {
		logCtx.WithField("err", err).Error("Unable to get outlet configs")
		return
	}

	// Temperature sensors
	temperatureSensors, err := helper.getMonitorSensorsTemp(baseRequest, device)
	if err != nil {
		logCtx.WithField("err", err).Error("Unable to get temperature sensors")
		return
	}

	// Humidity sensors
	humiditySensors, err := helper.getMonitorSensorsHumidity(baseRequest, device)
	if err != nil {
		logCtx.WithField("err", err).Error("Unable to get humidity sensors")
		return
	}

	// OutletGroups
	// This is pretty kludgey, but JAWS is wicked slow, so trying to turn off all the outlets (or even just a small
	// subset of them) can take a really long time. To facilitate the need to have a software EPO option, while I'm
	// in here I'm going to create an EPO group.
	// We want to create the EPO group, but we only want it to contain outlets belonging to this unit. So, build up
	// that list for this unit and then create the group.
	for _, unit := range units {
		thisUnitEPOGroupName := EPOOutletGroupPrefix + "_" + unit.Id
		thisUnitEPOGroup := OutletGroup{
			Name: thisUnitEPOGroupName,
		}
		for _, outlet := range outlets {
			if strings.HasPrefix(outlet.ID, unit.Id) {
				thisUnitEPOGroup.OutletAccess = append(thisUnitEPOGroup.OutletAccess, outlet.ID)
			}
		}

		epoPayload, marshalErr := json.Marshal(thisUnitEPOGroup)
		if marshalErr != nil {
			err = fmt.Errorf("unable to marshal EPO payload: %s", marshalErr)
			return
		}

		epoOutletGroupCreateRequest := baseRequest
		epoOutletGroupCreateRequest.FullURL = device.URL + "/config/groups/" + thisUnitEPOGroupName
		epoOutletGroupCreateRequest.Method = "POST"
		epoOutletGroupCreateRequest.ExpectedStatusCodes = []int{http.StatusCreated}
		epoOutletGroupCreateRequest.Payload = epoPayload

		_, _, createErr := epoOutletGroupCreateRequest.DoHTTPAction()
		if createErr != nil {
			if strings.HasSuffix(createErr.Error(), "409") {
				// This is kind of hacky, I really need to update the DoHTTPAction to return a real status code.
				// Anyway, 409 means conflict which means the group already exists. Thus, we will patch a new version.
				epoOutletGroupPatchRequest := epoOutletGroupCreateRequest
				epoOutletGroupPatchRequest.Method = "PATCH"
				epoOutletGroupPatchRequest.ExpectedStatusCodes = []int{http.StatusNoContent}

				_, _, patchErr := epoOutletGroupPatchRequest.DoHTTPAction()
				if patchErr != nil {
					err = fmt.Errorf("unable to patch EPO group: %s", createErr)
					return
				} else {
					logCtx.WithFields(log.Fields{
						"unit":             unit,
						"thisUnitEPOGroup": thisUnitEPOGroup,
					}).Debug("Updated EPO group for unit")
				}
			} else {
				err = fmt.Errorf("unable to create EPO group: %s", createErr)
				return
			}
		} else {
			logCtx.WithFields(log.Fields{
				"unit":             unit,
				"thisUnitEPOGroup": thisUnitEPOGroup,
			}).Debug("Created EPO group for unit")
		}
	}

	// Now we can go about our business as usual and get all of the groups.
	outletGroupsRequest := baseRequest
	outletGroupsRequest.FullURL = device.URL + "/config/groups"
	v, getErr = outletGroupsRequest.GetBodyForHTTPRequest()
	if v == nil {
		err = fmt.Errorf("unable to get body for outlet groups request: %s", getErr)
		return
	}

	outletGroupsInterface := v.([]interface{})
	var outletGroups []OutletGroup
	err = mapstructure.Decode(outletGroupsInterface, &outletGroups)
	if err != nil {
		logCtx.WithField("outletGroupsInterface", outletGroupsInterface).Error(
			"Unable to decode outletGroupsInterface")
		return
	}

	// Now that we have all the interfaces, loop through the units and assign as applicable.
	for unitIndex, _ := range units {
		unit := &units[unitIndex]

		// Branches
		for _, branch := range branches {
			if strings.HasPrefix(branch.ID, unit.Id) {
				unit.Branches = append(unit.Branches, branch)
			}
		}

		// Mains
		for _, line := range lines {
			if strings.HasPrefix(line.ID, unit.Id) {
				unit.Lines = append(unit.Lines, line)
			}
		}
		for _, phase := range phases {
			if strings.HasPrefix(phase.ID, unit.Id) {
				unit.Phases = append(unit.Phases, phase)
			}
		}

		// Outlets
		for _, outlet := range outlets {
			if strings.HasPrefix(outlet.ID, unit.Id) {
				unit.Outlets = append(unit.Outlets, outlet)
			}
		}

		// Outlet configs
		for _, outlet := range outletConfigs {
			if strings.HasPrefix(outlet.ID, unit.Id) {
				unit.OutletConfigs = append(unit.OutletConfigs, outlet)
			}
		}

		// Temperature Sensors
		for _, sensor := range temperatureSensors {
			if strings.HasPrefix(sensor.ID, unit.Id) {
				unit.TemperatureSensors = append(unit.TemperatureSensors, sensor)
			}
		}

		// Humidity Sensors
		for _, sensor := range humiditySensors {
			if strings.HasPrefix(sensor.ID, unit.Id) {
				unit.HumiditySensors = append(unit.HumiditySensors, sensor)
			}
		}

		// OutletGroups
		// If you're reading this you're probably wondering what the heck is going on in the next few lines...
		// These PDUs don't have the concept of a group of outlets belonging to only a single PDU. If you have 3 PDUs
		// you can create a group with outlets from all 3 of those PDUs. As such, this doesn't translate to the Redfish
		// way of thinking about outlet groups where a group belongs to only a PDU and can consist of only outlets from
		// that PDU.
		// So, here's what I decided to do: if every outlet in a group belongs to only one PDU, then only that PDU will
		// receive that group. Otherwise, as long as one outlet belongs to a given PDU, it will get that group.
		for _, outletGroup := range outletGroups {
			// Loop through all of the outlets in this group checking to see if any of the prefixs DON'T match.
			allMatch := true
			containsAtLeastOne := false
			for _, outlet := range outletGroup.OutletAccess {
				if strings.HasPrefix(outlet, unit.Id) {
					// It has the correct prefix, so we know that at LEAST one outlet matches.
					containsAtLeastOne = true
				} else {
					// They can't all match if just a single one doesn't.
					allMatch = false
				}
			}

			if containsAtLeastOne || allMatch {
				unit.OutletGroups = append(unit.OutletGroups, outletGroup)
			}
		}
	}

	// First make sure there aren't any keys for this xname
	removeErr := helper.RedisHelper.removeXnamePrependedKeys(xname)
	if removeErr != nil {
		err = fmt.Errorf("unable to remove keys from Redis: %s", removeErr)
		return
	}

	for _, unit := range units {
		// Populate Redis with this unit's information.
		err = helper.initRackPDU(xname, unit)
	}

	// Setup Manager collection
	err = helper.RedisHelper.initManagerCollection(xname)
	if err != nil {
		logCtx.WithError(err).Error("Failed to initialize Manager Collection for device")
		return
	}

	err = helper.RedisHelper.initManager(xname, GenericBmcID, "ServerTech JAWS")
	if err != nil {
		logCtx.WithError(err).Error("Failed to initialize Manager for device")
		return
	}

	// Setup Certificate Service for the device
	err = helper.CertificateService.InitForXName(xname)
	if err != nil {
		logCtx.WithError(err).Error("Failed to initialize Certificate Service for device")
		return
	}

	logCtx.WithFields(log.Fields{
		"device": device,
	}).Debug("Device initialized")

	// Create the DNS entry that will be used by HSM and friends to talk to this PDU. If this fails, no point in
	// continuing on or calling the rest of the process to this point a success as nobody will be able to talk to us.
	// The PDU will be getting the DNS name of x3000m0, so to differentiate RTS from the PDU will add the `-rts` suffix
	err = informDNS(ctx, xname+"-rts")
	if err != nil {
		return
	}

	// Grab the default RTS credentails from Vault
	creds, err := helper.RTSCredentialStore.GetGlobalCredentials()
	if err != nil {
		log.Panic(err)
	}

	// Add the credentials to talk to RTS to Vault so HSM knows how to handle it.
	cred := compcredentials.CompCredentials{
		Xname:    xname,
		URL:      xname + "-rts:8083",
		Username: creds.Username,
		Password: creds.Password,
	}
	compErr := compCredStore.StoreCompCred(cred)
	if compErr != nil {
		err = fmt.Errorf("unable to store credentials in Vault: %s", compErr)
		log.Error(err)
		return
	}

	log.WithField("xname", xname).Debug("Added credentials to Vault")

	// Now that all the necessary data is in Redis tell HSM we exist and have it discover us.
	hsmErr := informHSMWithFQDN(ctx, xname, cred.URL)
	if hsmErr != nil {
		err = fmt.Errorf("unable to inform HSM: %s", hsmErr)
		log.Error(err)
		return
	}

	log.WithField("xname", xname).Debug("Informed HSM")

	return
}

func (helper *JAWSBackedHelper) RunPeriodic(ctx context.Context, env map[string]interface{}) (err error) {
	// The interface being passed to us is a map of devices.
	devices, err := helper.PDUCredentialStore.GetDevices()
	if err != nil {
		log.WithField("err", err).Error("Unable to get devices.")
		return
	}

	// The problem with PDUs is we have no clue how many of them are connected together in the master/slave fashion,
	// and, we can't even really be sure that it's always going to be the same model with the same number of outlets.
	// So, in order to make any of the URLs work Redis needs to be pre-populated with discovered information specific
	// to each *master* PDU.
	var deviceWaitGroup sync.WaitGroup
	start := time.Now()

	for xname, device := range devices {
		// Check to see if we know this device exists already.
		if _, deviceIsKnown := helper.KnownDevices[xname]; deviceIsKnown {
			log.WithField("device", device).Trace("Skipping already known device")
			continue
		}

		log.WithField("device", device).Info("New device found, initiating discovery")

		deviceWaitGroup.Add(1)
		go func(xname string, device pdu_credential_store.Device) {
			defer deviceWaitGroup.Done()

			initErr := helper.initDevice(ctx, xname, device)

			if initErr != nil {
				err = fmt.Errorf("unable to initialize device: %s", initErr)
			} else {
				helper.KnownDevices[xname] = device

				elapsed := time.Since(start)
				log.WithFields(log.Fields{
					"device":   device,
					"initTime": elapsed,
				}).Debug("Done waiting for device to initialize")
			}
		}(xname, device)
	}
	deviceWaitGroup.Wait()

	return
}

func (helper *JAWSBackedHelper) SetupBackendHelper(ctx context.Context, env map[string]interface{}) (err error) {
	// For performance reasons we'll keep the client that was created for this base request and reuse it later.
	httpClient = retryablehttp.NewClient()
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient.HTTPClient.Transport = transport
	httpClient.HTTPClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// Time to setup the connection to Vault
	jawsVaultKeypath, ok := os.LookupEnv("JAWS_VAULT_KEYPATH")
	if !ok {
		log.Fatal("Environment variable JAWS_VAULT_KEYPATH was not set")
	}

	secureStorage := connectToVault()
	if secureStorage == nil {
		log.Fatal("Unable to connect to Vault")
	}

	log.WithFields(log.Fields{
		"jawsVaultKeypath": jawsVaultKeypath,
	}).Info("Connecting to Vault for JAWS...")

	helper.PDUCredentialStore = pdu_credential_store.NewPDUCredStore(jawsVaultKeypath, secureStorage)
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
		return
	}

	// Setup Polling
	jawsPollingEnabled, _ := os.LookupEnv("JAWS_POLLING_ENABLED")
	if strings.EqualFold(jawsPollingEnabled, "true") {
		helper.PollingEnabled = true
	}

	helper.PollingInterval = 10 * time.Second // Default Polling interval is 10 seconds
	if pollingIntervalRaw, ok := os.LookupEnv("JAWS_POLLING_INTERVAL"); ok {
		var interval int
		interval, err = strconv.Atoi(pollingIntervalRaw)
		if err != nil {
			log.WithFields(log.Fields{
				"error":              err,
				"pollingIntervalRaw": pollingIntervalRaw,
			}).Error("problem parsing JAWS_POLLING_INTERVAL")
			return
		}
		helper.PollingInterval = time.Duration(interval) * time.Second
	}

	helper.PollingTemperatureInterval = 30 * time.Second // Default polling interval for temperature is 30 seconds
	if intervalRaw, ok := os.LookupEnv("JAWS_POLLING_TEMPERATURE_INTERVAL"); ok {
		var interval int
		interval, err = strconv.Atoi(intervalRaw)
		if err != nil {
			log.WithFields(log.Fields{
				"error":       err,
				"intervalRaw": intervalRaw,
			}).Error("problem parsing JAWS_POLLING_TEMPERATURE_INTERVAL")
			return
		}

		helper.PollingTemperatureInterval = time.Duration(interval) * time.Second
	}

	helper.PollingHumidityInterval = 30 * time.Second // Default polling interval for humidity is 30 seconds
	if intervalRaw, ok := os.LookupEnv("JAWS_POLLING_HUMIDITY_INTERVAL"); ok {
		var interval int
		interval, err = strconv.Atoi(intervalRaw)
		if err != nil {
			log.WithFields(log.Fields{
				"error":       err,
				"intervalRaw": intervalRaw,
			}).Error("problem parsing JAWS_POLLING_HUMIDITY_INTERVAL")
			return
		}
		helper.PollingHumidityInterval = time.Duration(interval) * time.Second
	}

	helper.PollingWorkers = 30 // Default to useing 30 Polling workers, this is the current default of the Collector
	if pollingWorkersRaw, ok := os.LookupEnv("JAWS_POLLING_WORKERS"); ok {
		helper.PollingWorkers, err = strconv.Atoi(pollingWorkersRaw)
		if err != nil {
			log.WithFields(log.Fields{
				"error":             err,
				"pollingWorkersRaw": pollingWorkersRaw,
			}).Error("problem parsing JAWS_POLLING_WORKERS")
			return
		}
	}

	if helper.PollingEnabled {
		log.Info("Polling Enabled")
		go helper.doPolling(ctx)
	}

	// Certificate Service
	err = helper.CertificateService.SetupCertificateService(GenericBmcID, GenericCertificateID)
	if err != nil {
		log.WithError(err).Error("Failed to setup Certificate service")
		return
	}

	return
}

func (helper *JAWSBackedHelper) GetEnvForXname(xname string) (env map[string]string, err error) {
	env = map[string]string{}

	if device, ok := helper.KnownDevices[xname]; ok {
		env["RTS_URL"] = device.URL
		env["RTS_USERNAME"] = device.Username
		env["RTS_PASSWORD"] = device.Password
		env["RTS_XNAME"] = device.Xname

		return
	}

	err = errors.New("Unable to find xname in device list")
	log.WithField("xname", xname).Error(err)

	return
}

/*
 * Run
 */

func (helper *JAWSBackedHelper) RunBackendHelper(ctx context.Context, key string, args []string,
	env map[string]string) (value string, err error) {
	// We know we always need at least these 4 fields, so check for them right away.
	baseURL, ok := env["RTS_URL"]
	if !ok {
		err = errors.New("RTS_URL not in environment variables")
		log.Error(err.Error())
		return
	}

	username, ok := env["RTS_USERNAME"]
	if !ok {
		err = errors.New("RTS_USERNAME not in environment variables")
		log.Error(err.Error())
		return
	}

	password, ok := env["RTS_PASSWORD"]
	if !ok {
		err = errors.New("RTS_PASSWORD not in environment variables")
		log.Error(err.Error())
		return
	}

	xname, ok := env["RTS_XNAME"]
	if !ok {
		err = errors.New("RTS_XNAME not in environment variables")
		log.Error(err.Error())
		return
	}

	// Check to make sure we actually know about this device.
	if _, deviceIsKnown := helper.KnownDevices[xname]; !deviceIsKnown {
		err = fmt.Errorf("unknown xname: %s", xname)
		log.Error(err.Error())
		return
	}

	// Do some sanity checks to make sure we're getting what we expect.
	// Every key that comes in could be xname prefixed, use a regular expression to capture that.
	keyRegex := regexp.MustCompile(`(x\d+p\d+){0,}(` + RackPDUsKeyspace + `\S+$)`)
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

	pduPrefixedKey := strings.TrimPrefix(fullKey, RackPDUsKeyspace)
	// If the trim didn't work it will be the same as the key and we don't want that.
	if pduPrefixedKey == key {
		err = errors.New("unexpected PDU key prefix")
		log.WithField(
			"pduPrefixedKey", pduPrefixedKey,
		).Error("Unable to derive the PDU prefixed key")
		return
	}

	// At this point every pduPrefixedKey is of the form:
	//   /PDU_ID/Resource/RESOURCE_ID/etc.
	// For example:
	//   /A/Resource/AA3/Actions/Outlet.PowerControl
	// or:
	//   /A/Name
	// What we need now is the PDU ID and the the rest.
	// Go is pretty cool, it has this built-in function called SplitN, which will return a max of N substrings
	// so we don't over split.
	keyParts := strings.SplitN(strings.TrimPrefix(pduPrefixedKey, "/"), "/", 2)
	if len(keyParts) != 2 {
		err = errors.New("pduPrefixedKey does not contain the right number of parts")
		log.WithField("pduPrefixedKey", pduPrefixedKey).Error("Unable to split pduPrefixedKey")
		return
	}

	pduID := keyParts[0]
	deviceSpecificKey := keyParts[1]

	// Figure out what URL we need to go to.
	var op string
	var urlPath string
	var payload []byte
	var invalidatedKeys []string
	expectedStatusCode := http.StatusOK

	if strings.HasPrefix(deviceSpecificKey, "Status") {
		op = "GET"
		urlPath = "/monitor/units"
	} else if strings.HasPrefix(deviceSpecificKey, "Branches") {
		// There are multiple URLs that could get us here, need to split up and see what's being asked for.
		branchURL := strings.TrimPrefix(deviceSpecificKey, "Branches/")
		branchURLParts := strings.Split(branchURL, "/")

		// Better always have a branch
		branch := branchURLParts[0]

		op = "GET"
		urlPath = filepath.Join("/monitor/branches", branch)
	} else if strings.HasPrefix(deviceSpecificKey, "Mains") {
		mainURL := strings.TrimPrefix(deviceSpecificKey, "Mains/")
		mainURLParts := strings.Split(mainURL, "/")

		op = "GET"

		// Since multiple results combine together to get one sensor, we have to get the whole collection every time.
		if mainURLParts[1] == "PolyPhaseCurrentSensors" {
			urlPath = "/monitor/lines"
		} else if mainURLParts[1] == "PolyPhasePowerSensors" {
			urlPath = "/monitor/phases"
		}
	} else if strings.HasPrefix(deviceSpecificKey, "Outlets") {
		// There are multiple URLs that could get us here, need to split up and see what's being asked for.
		outletURL := strings.TrimPrefix(deviceSpecificKey, "Outlets/")
		outletURLParts := strings.Split(outletURL, "/")

		// Better always have an outlet
		outlet := outletURLParts[0]

		var jawsOutlet string
		jawsOutlet, err = translateRedfishToJAWSOutlet(outletURLParts[0])
		if err != nil {
			return
		}

		if outletURLParts[1] == "Actions" {
			// /redfish/v1/PowerEquipment/RackPDUs/1/Outlets/AA1/Actions/Outlet.PowerControl
			op = "PATCH"
			expectedStatusCode = http.StatusNoContent

			action := outletURLParts[2]

			if action == "Outlet.PowerControl" {
				urlPath = filepath.Join("/control/outlets", jawsOutlet)
				postBody := make(map[string]string)
				postBody["control_action"], ok = env["PowerState"]
				if !ok {
					err = errors.New("PowerControl action requested, but PowerState not provided")
					log.Error(err.Error())

					return
				}

				payload, err = json.Marshal(postBody)
				if err != nil {
					err = errors.New("unable to marshal Outlet.PowerControl payload")
					log.WithField("postBody", postBody).Error(err.Error())
					return
				}

				powerStateKey := filepath.Join(xname, RackPDUsKeyspace, pduID, "Outlets", outlet, "PowerState")
				invalidatedKeys = append(invalidatedKeys, powerStateKey)
			} else {
				err = errors.New("unknown action for Outlets")
				log.WithFields(log.Fields{
					"action": action,
				}).Error(err.Error())
				return
			}
		} else if outletURLParts[1] == "PowerRestorePolicy" {
			op = "GET"
			urlPath = filepath.Join("/config/outlets", jawsOutlet)
		} else {
			op = "GET"
			urlPath = filepath.Join("/monitor/outlets", jawsOutlet)
		}
	} else if strings.HasPrefix(deviceSpecificKey, "OutletGroups") {
		// There are multiple URLs that could get us here, need to split up and see what's being asked for.
		outletGroupURL := strings.TrimPrefix(deviceSpecificKey, "OutletGroups/")
		outletGroupURLParts := strings.Split(outletGroupURL, "/")

		// Better always have an outlet group
		outletGroup := outletGroupURLParts[0]

		if outletGroupURLParts[1] == "Actions" {
			// /redfish/v1/PowerEquipment/RackPDUs/A/OutletGroups/CrayEPO_A/Actions/OutletGroup.PowerControl
			op = "PATCH"
			expectedStatusCode = http.StatusNoContent

			action := outletGroupURLParts[2]

			if action == "OutletGroup.PowerControl" {
				urlPath = filepath.Join("/control/groups", outletGroup)
				postBody := make(map[string]string)
				postBody["control_action"], ok = env["PowerState"]
				if !ok {
					err = errors.New("PowerControl action requested, but PowerState not provided")
					log.Error(err.Error())

					return
				}

				payload, err = json.Marshal(postBody)
				if err != nil {
					err = errors.New("unable to marshal OutletGroup.PowerControl payload")
					log.WithField("postBody", postBody).Error(err.Error())
					return
				}
			} else {
				err = errors.New("unknown action for Outlets")
				log.WithFields(log.Fields{
					"action": action,
				}).Error(err.Error())
				return
			}
		} else {
			err = errors.New("unknown OutletGroup key")
			log.WithField(
				"deviceSpecificKey", deviceSpecificKey,
			).Error("Unknown OutletGroup key")
			return
		}
	} else if strings.HasPrefix(deviceSpecificKey, "Sensors/Temperature") {
		// There are multiple URLs that could get us here, need to split up and see what's being asked for.
		sensorURL := strings.TrimPrefix(deviceSpecificKey, "Sensors/Temperature")
		sensorURLParts := strings.Split(sensorURL, "/")

		// Better always have a temperature sensors
		sensor := sensorURLParts[0]

		op = "GET"
		urlPath = filepath.Join("/monitor/sensors/temp", sensor)

		log.WithFields(log.Fields{
			"sensor":    sensor,
			"sensorURL": sensorURL,
			"urlPath":   urlPath,
		}).Info("Temperature Handler - start")

	} else if strings.HasPrefix(deviceSpecificKey, "Sensors/Humidity") {
		// There are multiple URLs that could get us here, need to split up and see what's being asked for.
		sensorURL := strings.TrimPrefix(deviceSpecificKey, "Sensors/Humidity")
		sensorURLParts := strings.Split(sensorURL, "/")

		// Better always have a temperature sensors
		sensor := sensorURLParts[0]

		op = "GET"
		urlPath = filepath.Join("/monitor/sensors/humidity", sensor)
	} else {
		err = errors.New("unknown key")
		log.WithField(
			"deviceSpecificKey", deviceSpecificKey,
		).Error("Unknown key")
		return
	}

	// Check to make sure that the urlPath actually got set to something.
	if urlPath == "" {
		log.WithFields(log.Fields{
			"key":     key,
			"op":      op,
			"payload": string(payload),
		}).Error("urlPath empty")
		err = errors.New("unable to build full url")
		return
	}

	auth := hmshttp.Auth{
		Username: username,
		Password: password,
	}
	request := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		Timeout:             10 * time.Second,
		ExpectedStatusCodes: []int{expectedStatusCode},
		Auth:                &auth,
		FullURL:             baseURL + urlPath,
		Method:              op,
		CustomHeaders:       getSvcInstName(),
		Payload:             payload,
	}

	logFields := log.Fields{
		"request": request,
	}

	v, err := request.GetBodyForHTTPRequest()
	if err != nil {
		log.WithFields(log.Fields{
			"err":             err,
			"request.FullURL": request.FullURL,
			"request.Method":  request.Method,
		}).Error("HTTP Request Failed")
		return
	}
	if v != nil {
		pl := helper.RedisHelper.startPipeline()

		// Now switch on the URL to determine the right function to use to get the appropriate data member.
		if urlPath == "/monitor/units" {
			unitsInterface := v.([]interface{})
			var units []MonitorUnit
			decodeErr := mapstructure.Decode(unitsInterface, &units)
			if decodeErr != nil {
				log.WithField("unitsInterface", unitsInterface).Error(
					"Unable to decode Monitor Units interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			populateErr := helper.populateRedisForMonitorUnits(pl, units, xname, pduID)
			if populateErr != nil {
				log.WithField("units", units).Error(
					"Unable to populate Redis with Monitor Units information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/monitor/lines") {
			linesInterface := v.([]interface{})
			var lines []Line
			decodeErr := mapstructure.Decode(linesInterface, &lines)
			if decodeErr != nil {
				log.WithField("linesInterface", linesInterface).Error(
					"Unable to decode Lines interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			populateErr := helper.populateRedisForLines(pl, lines, xname, pduID)
			if populateErr != nil {
				log.WithField("lines", lines).Error("Unable to populate Redis with Lines information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/monitor/phases") {
			phasesInterface := v.([]interface{})
			var phases []Phase
			decodeErr := mapstructure.Decode(phasesInterface, &phases)
			if decodeErr != nil {
				log.WithField("phasesInterface", phasesInterface).Error(
					"Unable to decode Phases interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			populateErr := helper.populateRedisForPhases(pl, phases, xname, pduID)
			if populateErr != nil {
				log.WithField("phases", phases).Error("Unable to populate Redis with Phases information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/monitor/outlets") {
			outletInterface := v.(map[string]interface{})
			var outlet MonitorOutlet
			decodeErr := mapstructure.Decode(outletInterface, &outlet)
			if decodeErr != nil {
				log.WithField("outletInterface", outletInterface).Error(
					"Unable to decode Outlet interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			populateErr := helper.populateRedisForMonitorOutlet(pl, outlet, xname, pduID)
			if populateErr != nil {
				log.WithField("outlet", outlet).Error("Unable to populate Redis with Outlet information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/monitor/branches") {
			branchInterface := v.(map[string]interface{})
			var branch Branch
			decodeErr := mapstructure.Decode(branchInterface, &branch)
			if decodeErr != nil {
				log.WithField("branchInterface", branchInterface).Error(
					"Unable to decode Branch interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			populateErr := helper.populateRedisForBranch(pl, branch, xname, pduID)
			if populateErr != nil {
				log.WithField("branch", branch).Error("Unable to populate Redis with Branch information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/config/outlets") {
			outletInterface := v.(map[string]interface{})
			var outlet ConfigOutlet
			decodeErr := mapstructure.Decode(outletInterface, &outlet)
			if decodeErr != nil {
				log.WithField("outletInterface", outletInterface).Error(
					"Unable to decode Outlet interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			populateErr := helper.populateRedisForConfigOutlet(pl, outlet, xname, pduID)
			if populateErr != nil {
				log.WithField("outlet", outlet).Error("Unable to populate Redis with Outlet information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/monitor/sensors/temp") {
			log.WithFields(log.Fields{
				"urlPath": urlPath,
			}).Info("Temperature Handler - end")

			sensorInterface := v.(map[string]interface{})
			var sensor TemperatureSensor
			decodeErr := mapstructure.Decode(sensorInterface, &sensor)
			if decodeErr != nil {
				log.WithField("outletInterface", sensorInterface).Error(
					"Unable to decode Temperature Sensor interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			sensors := []TemperatureSensor{sensor}
			populateErr := helper.populateRedisForTemperature(pl, sensors, xname, pduID)
			if populateErr != nil {
				log.WithField("outlet", sensor).Error("Unable to populate Redis with Temperature Sensor information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else if strings.HasPrefix(urlPath, "/monitor/sensors/humidity") {
			sensorInterface := v.(map[string]interface{})
			var sensor HumiditySensor
			decodeErr := mapstructure.Decode(sensorInterface, &sensor)
			if decodeErr != nil {
				log.WithField("outletInterface", sensorInterface).Error(
					"Unable to decode Temperature Sensor interface to struct")
				err = errors.New("unable to decode fetched data")
				return
			}

			sensors := []HumiditySensor{sensor}
			populateErr := helper.populateRedisForHumidity(pl, sensors, xname, pduID)
			if populateErr != nil {
				log.WithField("outlet", sensor).Error("Unable to populate Redis with Humidity Sensor information")
				err = errors.New("unable to populate database with fetched data")
				return
			}
		} else {
			// Anything else we just don't care (nothing to return).
			log.WithFields(logFields).Trace("Completed HTTP action without return")

			return
		}

		_, err = pl.Exec()
		if err != nil {
			return
		}
	} else {
		logFields["invalidatedKeys"] = invalidatedKeys

		// If v is nil then we got nothing back which indicates we PATCHed.
		log.WithFields(logFields).Trace("Completed HTTP PATCH")

		// Good chance we altered some state with whatever PATCH we just did, best check if there are any invalidated
		// keys that no longer reflect the state of the system.
		helper.RedisHelper.invalidateRedisKeys(invalidatedKeys)

		// For outlet control we want to POST a RF event to the collector, so call that out specifically.
		if strings.HasPrefix(urlPath, "/control/outlets") {
			// Get back the POST'd body.
			var postBody map[string]string
			unmarshalErr := json.Unmarshal(request.Payload, &postBody)
			if unmarshalErr != nil {
				log.WithField("unmarshalErr", unmarshalErr).Error("Unable to unmarshal POST'd payload")
				return
			}

			newPostState, ok := postBody["control_action"]
			if !ok {
				log.WithField("postBody", postBody).Error(
					"Unable to retreive control_action from POST'd body")
				return
			}

			// Right now the key looks like this:
			//   x0m0/redfish/v1/PowerEquipment/RackPDUs/A/Outlets/AA3/PowerState
			// We need to whack off the front and end of that to look like this:
			//  /redfish/v1/PowerEquipment/RackPDUs/A/Outlets/AA3
			if invalidatedKeys != nil && len(invalidatedKeys) > 0 {
				resource := strings.TrimPrefix(invalidatedKeys[0], xname)
				resource = strings.TrimSuffix(resource, "/PowerState")

				_ = postRFPowerEvent(ctx, xname, resource, newPostState)
			} else {
				log.Error("Unable to determine affected outlet for event because key not set")
				return
			}
		}

		return
	}

	value, err = helper.RedisHelper.Redis.Get(key).Result()
	logFields[value] = value
	log.WithFields(logFields).Trace("Completed HTTP action")

	return
}

func (helper *JAWSBackedHelper) disableDHCPStaticAddressFallback(request hmshttp.HTTPRequest, device pdu_credential_store.Device) error {
	type patchConfigNetworkRequest struct {
		DHCPStaticAddressFallBack string `json:"dhcp_static_address_fallback"`
	}

	requestPayload := patchConfigNetworkRequest{
		DHCPStaticAddressFallBack: "disabled",
	}

	payload, err := json.Marshal(requestPayload)
	if err != nil {
		return err
	}

	request.FullURL = device.URL + "/config/network"
	request.Method = "PATCH"
	request.ExpectedStatusCodes = []int{http.StatusNoContent}
	request.Payload = payload

	_, _, patchErr := request.DoHTTPAction()

	return patchErr
}

func (helper *JAWSBackedHelper) getMonitorUnits(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]MonitorUnit, error) {
	request.FullURL = device.URL + "/monitor/units"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for units request: %s", getErr)
	}

	unitsInterface := v.([]interface{})
	var units []MonitorUnit
	err := mapstructure.Decode(unitsInterface, &units)
	if err != nil {
		log.WithFields(log.Fields{
			"unitsInterface": unitsInterface,
			"err":            err,
		}).Error("Unable to decode unitsInterface")
		return nil, err
	}

	return units, nil
}

func (helper *JAWSBackedHelper) getConfigOutlets(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]ConfigOutlet, error) {
	request.FullURL = device.URL + "/config/outlets"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for config outlets request: %s", getErr)
	}

	outletInterface := v.([]interface{})
	var outlets []ConfigOutlet
	err := mapstructure.Decode(outletInterface, &outlets)
	if err != nil {
		log.WithFields(log.Fields{
			"outletInterface": outletInterface,
			"err":             err,
		}).Error("Unable to decode outletInterface")
		return nil, err
	}

	return outlets, nil
}

func (helper *JAWSBackedHelper) getMonitorOutlets(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]MonitorOutlet, error) {
	request.FullURL = device.URL + "/monitor/outlets"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for monitor outlets request: %s", getErr)
	}

	outletInterface := v.([]interface{})
	var outlets []MonitorOutlet
	err := mapstructure.Decode(outletInterface, &outlets)
	if err != nil {
		log.WithFields(log.Fields{
			"outletInterface": outletInterface,
			"err":             err,
		}).Error("Unable to decode Outlet interface to struct")
		return nil, errors.New("unable to decode fetched data")
	}

	return outlets, nil
}

func (helper *JAWSBackedHelper) getMonitorBranches(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]Branch, error) {
	request.FullURL = device.URL + "/monitor/branches"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for monitor branches request: %s", getErr)
	}

	branchInterface := v.([]interface{})
	var branches []Branch
	err := mapstructure.Decode(branchInterface, &branches)
	if err != nil {
		log.WithFields(log.Fields{
			"branchInterface": branchInterface,
			"err":             err,
		}).Error("Unable to decode Branch interface to struct")
		return nil, errors.New("unable to decode fetched data")
	}

	return branches, nil
}

func (helper *JAWSBackedHelper) getMonitorPhases(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]Phase, error) {
	request.FullURL = device.URL + "/monitor/phases"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for monitor phases request: %s", getErr)
	}

	phasesInterface := v.([]interface{})
	var phases []Phase
	err := mapstructure.Decode(phasesInterface, &phases)
	if err != nil {
		log.WithFields(log.Fields{
			"phasesInterface": phasesInterface,
			"err":             err,
		}).Error("Unable to decode phases interface to struct")
		return nil, errors.New("unable to decode fetched data")
	}

	return phases, nil
}

func (helper *JAWSBackedHelper) getMonitorLines(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]Line, error) {
	request.FullURL = device.URL + "/monitor/lines"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for monitor lines request: %s", getErr)
	}

	linesInterface := v.([]interface{})
	var lines []Line
	err := mapstructure.Decode(linesInterface, &lines)
	if err != nil {
		log.WithFields(log.Fields{
			"linesInterface": linesInterface,
			"err":            err,
		}).Error("Unable to decode lines interface to struct")
		return nil, errors.New("unable to decode fetched data")
	}

	return lines, nil
}

func (helper *JAWSBackedHelper) getMonitorSensorsTemp(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]TemperatureSensor, error) {
	request.FullURL = device.URL + "/monitor/sensors/temp"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for monitor sensor temperature request: %s", getErr)
	}

	sensorsInterface := v.([]interface{})
	var sensors []TemperatureSensor
	err := mapstructure.Decode(sensorsInterface, &sensors)
	if err != nil {
		log.WithFields(log.Fields{
			"sensorsInterface": sensorsInterface,
			"err":              err,
		}).Error("Unable to decode temperature sensors interface to struct")
		return nil, errors.New("unable to decode fetched data")
	}

	return sensors, nil
}

func (helper *JAWSBackedHelper) getMonitorSensorsHumidity(request hmshttp.HTTPRequest, device pdu_credential_store.Device) ([]HumiditySensor, error) {
	request.FullURL = device.URL + "/monitor/sensors/humidity"

	v, getErr := request.GetBodyForHTTPRequest()
	if v == nil {
		return nil, fmt.Errorf("unable to get body for monitor sensor humidity request: %s", getErr)
	}

	sensorsInterface := v.([]interface{})
	var sensors []HumiditySensor
	err := mapstructure.Decode(sensorsInterface, &sensors)
	if err != nil {
		log.WithFields(log.Fields{
			"sensorsInterface": sensorsInterface,
			"err":              err,
		}).Error("Unable to decode humidity sensors interface to struct")
		return nil, errors.New("unable to decode fetched data")
	}

	return sensors, nil
}
