// Copyright 2020 Hewlett Packard Enterprise Development LP

package backend_helpers

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	hmshttp "stash.us.cray.com/HMS/hms-go-http-lib"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/pdu_credential_store"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/telemetry"
)

func (helper JAWSBackedHelper) doPolling(ctx context.Context) {
	log.WithFields(log.Fields{
		"pollingWorkers":             helper.PollingWorkers,
		"pollingInterval":            helper.PollingInterval,
		"pollingTemperatureInterval": helper.PollingTemperatureInterval,
		"pollingHumidityInterval":    helper.PollingHumidityInterval,
	}).Info("Collecting data from endpoints at interval")

	// Setup channels for pendingEndpoints and Event payloads.
	pendingEndpoints := make(chan pdu_credential_store.Device, helper.PollingWorkers)
	eventPayloads := make(chan telemetry.Event, 10000)

	// Start up the pool of workers
	helper.CurrentPolling = NewXNameSet()
	for worker := 1; worker <= helper.PollingWorkers; worker++ {
		go helper.collectData(ctx, worker, pendingEndpoints, eventPayloads)
		go helper.sendTelemetryEvents(ctx, worker, eventPayloads)
	}

	for true {
		for _, endpoint := range helper.KnownDevices {
			pendingEndpoints <- endpoint
		}
		time.Sleep(helper.PollingInterval)
	}
}

func (helper *JAWSBackedHelper) collectData(ctx context.Context, id int, pendingEndpoints <-chan pdu_credential_store.Device, eventPayloads chan<- telemetry.Event) {
	log.WithField("worker", id).Info("Starting polling Worker")
	for endpoint := range pendingEndpoints {
		// Check that we are not currently polling the PDU. It is important that we prevent making too many requests to
		// the PDU simultaneously, otherwise there is a good chance that PDU can fall over.
		if helper.CurrentPolling.Get(endpoint.Xname) {
			log.WithField("xname", endpoint.Xname).Warn("PDU is currently being polled, skipping")
			continue
		}

		helper.CurrentPolling.Set(endpoint.Xname, true)
		helper.pollPDU(ctx, endpoint, eventPayloads)
		helper.CurrentPolling.Set(endpoint.Xname, false)
	}
}

func (helper *JAWSBackedHelper) sendTelemetryEvents(ctx context.Context, id int, eventPayloads <-chan telemetry.Event) {
	log.WithField("worker", id).Info("Starting Telemetry Worker")

	for event := range eventPayloads {
		if len(event.Events[0].Oem.Sensors) == 0 {
			log.WithField(
				"eventContext", event.Context,
			).Error("Empty Telemetry Event")
			continue
		}

		if err := postTelemetryEvent(ctx, event); err != nil {
			log.WithField("err", err).Error("Failed to post telemetry event")
		}
	}
}

func (helper *JAWSBackedHelper) pollPDU(ctx context.Context, device pdu_credential_store.Device, eventPayloads chan<- telemetry.Event) {
	xname := device.Xname
	logCtx := log.WithField("xname", xname)

	logCtx.Info("Polling PDU")

	auth := hmshttp.Auth{
		Username: device.Username,
		Password: device.Password,
	}

	baseRequest := hmshttp.HTTPRequest{
		Client:              httpClient,
		Context:             ctx,
		Timeout:             10 * time.Second,
		ExpectedStatusCodes: []int{http.StatusOK},
		Auth:                &auth,
		Method:              http.MethodGet,
	}

	// Profile how long we are interacting with Redis and how long it takes to poll the PDU overall
	var redisTime time.Duration
	start := time.Now()

	// We can use a timeout key to determine when we need to poll the PDU for /monitor/units
	// This information we only need to get once every minute.
	unitsKey := path.Join(xname, "poll", "monitor", "units")
	if poll, err := helper.RedisHelper.checkTimeoutKey(unitsKey, time.Minute); err == nil && poll {
		// Units Request - /monitor/units
		// KeySpace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Status/Health
		units, err := helper.getMonitorUnits(baseRequest, device)
		if err != nil {
			logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/units")
			return
		}

		var pduIDs []string
		begin := time.Now()
		pl := helper.RedisHelper.startPipeline()
		for _, unit := range units {
			pduIDs = append(pduIDs, unit.Id)
			helper.populateRedisForMonitorUnits(pl, units, xname, unit.Id)
		}
		if _, err := pl.Exec(); err != nil {
			logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
		}
		redisTime += time.Now().Sub(begin)
	} else if err != nil {
		logCtx.WithField("err", err).Error("Unable to check key")
		return
	}

	// Look up the PDU IDs underneath this PDU Controller
	rackPDUCollection := path.Join(xname, "redfish", "v1", "PowerEquipment", "RackPDUs", "Members")
	pduIDs, err := helper.RedisHelper.collectionMembers(rackPDUCollection)
	if err != nil {
		logCtx.WithFields(log.Fields{
			"rackPDUsKey": rackPDUCollection,
			"err":         err,
		}).Error("Unable to get PDU IDs")
		return
	}

	// Set up the Telemetry Helper once we know the PDU IDs available with this PDU Controller
	tHelper := JAWsTelemetryHelper{
		pduControllerXName: xname,
		pduIDs:             pduIDs,
		eventPayloads:      eventPayloads,
	}

	// Phases - /monitor/phases
	// KeySpace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Mains/AC1/PolyPhasePowerSensors/{branch_id}
	if phases, err := helper.getMonitorPhases(baseRequest, device); err == nil {
		// Send Phase Telemetry events
		tHelper.SendPhaseEvents(phases, time.Now())

		// Update Phase data in Redis
		begin := time.Now()
		pl := helper.RedisHelper.startPipeline()
		for _, pduID := range pduIDs {
			helper.populateRedisForPhases(pl, phases, xname, pduID)
		}
		if _, err := pl.Exec(); err != nil {
			logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
		}
		redisTime += time.Now().Sub(begin)
	} else {
		logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/phases")
	}

	// Lines /monitor/lines
	// KeySpace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Mains/AC1/PolyPhaseCurrentSensors/{line_id}
	if lines, err := helper.getMonitorLines(baseRequest, device); err == nil {
		// Send Line Telemetry events
		tHelper.SendLineEvents(lines, time.Now())

		// Update Line data in Redis
		begin := time.Now()
		pl := helper.RedisHelper.startPipeline()
		for _, pduID := range pduIDs {
			helper.populateRedisForLines(pl, lines, xname, pduID)
		}
		if _, err := pl.Exec(); err != nil {
			logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
		}
		redisTime += time.Now().Sub(begin)
	} else {
		logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/lines")
	}

	// Monitor Outlets -  /monitor/outlets
	// KeySpace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Outlets/{outlet_id}
	if monitorOutlets, err := helper.getMonitorOutlets(baseRequest, device); err == nil {
		// Send Outlet Telemetry events
		tHelper.SendOutletEvents(monitorOutlets, time.Now())

		// Update Outlet data in Redis
		begin := time.Now()
		pl := helper.RedisHelper.Redis.Pipeline()
		for _, outlet := range monitorOutlets {
			pduID := outlet.ID[0:1]
			helper.populateRedisForMonitorOutlet(pl, outlet, xname, pduID)
		}
		if _, err := pl.Exec(); err != nil {
			logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
		}
		redisTime += time.Now().Sub(begin)

	} else {
		logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/outlets")
	}

	// Branches - /monitor/branches
	// KeySpace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Branches/{branch_id}
	if branches, err := helper.getMonitorBranches(baseRequest, device); err == nil {
		// Send Branch Telemetry events
		tHelper.SendBranchEvents(branches, time.Now())

		// Update Branch data in Redis
		begin := time.Now()
		pl := helper.RedisHelper.startPipeline()
		for _, branch := range branches {
			pduID := branch.ID[0:1]
			helper.populateRedisForBranch(pl, branch, xname, pduID)
		}
		if _, err := pl.Exec(); err != nil {
			logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
		}
		redisTime += time.Now().Sub(begin)
	} else {
		logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/branches")
	}

	// We can use a timeout key to determine when we need to poll the PDU for /monitor/sensors/temp
	temperaturePollRate := helper.PollingTemperatureInterval
	temperatureKey := path.Join(xname, "poll", "monitor", "temperature")
	if poll, err := helper.RedisHelper.checkTimeoutKey(temperatureKey, temperaturePollRate); err == nil && poll {
		// Temperature - /monitor/sensors/temp
		// Keyspace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Sensors/Temperature{sensors_id}
		if temperatures, err := helper.getMonitorSensorsTemp(baseRequest, device); err == nil {
			// Send Temperature Telemetry events
			tHelper.SendTemperatureEvents(temperatures, time.Now())

			// Update Phase data in Redis
			begin := time.Now()
			pl := helper.RedisHelper.startPipeline()
			for _, pduID := range pduIDs {
				helper.populateRedisForTemperature(pl, temperatures, xname, pduID)
			}
			if _, err := pl.Exec(); err != nil {
				logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
			}
			redisTime += time.Now().Sub(begin)
		} else {
			logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/sensors/temp")
		}
	} else if err != nil {
		logCtx.WithFields(log.Fields{
			"err":            err,
			"temperatureKey": temperatureKey,
		}).Error("Unable to check key")
	}

	// We can use a timeout key to determine when we need to poll the PDU for /monitor/sensors/humidity
	humidityPollRate := helper.PollingHumidityInterval
	humidityKey := path.Join(xname, "poll", "monitor", "humidity")
	if poll, err := helper.RedisHelper.checkTimeoutKey(humidityKey, humidityPollRate); err == nil && poll {
		// Temperature - /monitor/sensors/humidity
		// Keyspace: /redfish/v1/PowerEquipment/RackPDUs/{pdu_id}/Sensors/Humidity{sensors_id}
		if humiditySensors, err := helper.getMonitorSensorsHumidity(baseRequest, device); err == nil {
			// Send Humidity Telemetry events
			tHelper.SendHumidityEvents(humiditySensors, time.Now())

			// Update Phase data in Redis
			begin := time.Now()
			pl := helper.RedisHelper.startPipeline()
			for _, pduID := range pduIDs {
				helper.populateRedisForHumidity(pl, humiditySensors, xname, pduID)
			}
			if _, err := pl.Exec(); err != nil {
				logCtx.WithField("err", err).Error("Unable to Exec() pipeline")
			}
			redisTime += time.Now().Sub(begin)
		} else {
			logCtx.WithField("err", err).Error("Unable to poll PDU for /monitor/sensors/humidity")
		}
	} else if err != nil {
		logCtx.WithFields(log.Fields{
			"err":         err,
			"humidityKey": humidityKey,
		}).Error("Unable to check key")
	}

	delta := time.Now().Sub(start)
	logCtx.WithFields(log.Fields{
		"delta":     delta,
		"redisTime": redisTime,
	}).Info("Polling round finished")
}

type JAWsTelemetryHelper struct {
	pduIDs             []string
	pduControllerXName string
	eventPayloads      chan<- telemetry.Event
}

func (helper *JAWsTelemetryHelper) parseIndex(id string) (uint8, error) {
	// The following JAWs IDs (with examples) are supported:
	// Outlet ID: BA36, or BA1
	// Phase ID: AA1
	// Line ID: AA1
	// Branch ID: BA6
	outletIndexRaw := getIndexFromJAWsID(id)
	outletIndex, err := strconv.Atoi(outletIndexRaw)
	if err != nil {
		return 0, fmt.Errorf("Unable to parse outlet index")
	}

	return uint8(outletIndex), nil
}

func (helper *JAWsTelemetryHelper) pduXName(id string) (string, error) {
	// For the given JAWs ID determine the xname of the associated PDU.

	// Sort the PDU IDs lexigraphy. This is the method that SMD follows when it generates Ordinals
	// for the PDU xnames.
	sort.Strings(helper.pduIDs)

	for ordinal, pduID := range helper.pduIDs {
		if strings.HasPrefix(id, pduID) {
			pduXName := fmt.Sprintf("%sp%d", helper.pduControllerXName, ordinal)
			return pduXName, nil
		}
	}

	return "", fmt.Errorf("Unable to determine PDU XName for given id %s", id)
}

func (helper *JAWsTelemetryHelper) stripUnitPrefix(value string) (string, error) {
	// Branch and Line events contain a prefix to idetntity which PDU unit they are associated with.
	// For example the following names are used: BA:L3 or BA:L3-L1-BR6. The A units have the prefix AA:, and
	// the B units have the prefix BA:.
	s := strings.SplitN(value, ":", 2)

	if len(s) != 2 {
		return "", fmt.Errorf("unable to strip unit prefix")
	}

	return s[1], nil
}

func (helper *JAWsTelemetryHelper) newEvent(timestamp time.Time, sensorType telemetry.CrayTelemetryType, sensors []telemetry.CraySensorPayload) telemetry.Event {
	return telemetry.Event{
		Context: helper.pduControllerXName + ":" + string(sensorType),
		Events: []telemetry.EventRecord{
			{
				MessageId:   string(sensorType),
				Message:     "See Oem/CrayPayload property",
				MessageArgs: []string{},

				EventTimestamp: timestamp.UTC().Format(time.RFC3339Nano),
				Oem: &telemetry.Sensors{
					TelemetrySource: "River",
					Sensors:         sensors,
				},
			},
		},
		EventsOCount: 1, // There is always 1 event record
	}
}

func (helper *JAWsTelemetryHelper) OutletEvent(outlets []MonitorOutlet, tType telemetry.CrayTelemetryType, timestamp time.Time) (telemetry.Event, error) {
	// Sensor identification:
	//	- Location is the PDU xname, such as x0m0p0
	//	- Physical context is always Outlet
	//	- Sensor index is the id number of the Outlet, Outlet 2 has the index 2

	var sensors []telemetry.CraySensorPayload

	for _, outlet := range outlets {
		pduXName, err := helper.pduXName(outlet.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		index, err := helper.parseIndex(outlet.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		var sensorValue string

		switch tType {
		case telemetry.Voltage:
			// Current is measured in Volts
			sensorValue = fmt.Sprintf("%f", outlet.Voltage)
		case telemetry.Current:
			// Current is measured in Amps
			sensorValue = fmt.Sprintf("%f", outlet.Current)
		case telemetry.Energy:
			// Energy is given as Watt-Hours from JAWS, Cray telemetry expects Joules
			// Conversion: 1 Watt Hour == 3600 Joules
			energy := outlet.Energy * 3600
			sensorValue = fmt.Sprintf("%d", energy)
		default:
			return telemetry.Event{}, fmt.Errorf("Invalid telemetry type: %s", tType)
		}

		sensor := telemetry.CraySensorPayload{
			Timestamp:          timestamp.UTC().Format(time.RFC3339Nano),
			Location:           pduXName,
			Index:              &index,
			Value:              sensorValue,
			ParentalContext:    "PDU",
			PhysicalContext:    "Outlet",
			PhysicalSubContext: "Output",
		}

		sensors = append(sensors, sensor)
	}

	return helper.newEvent(timestamp, tType, sensors), nil
}

func (helper *JAWsTelemetryHelper) SendOutletEvents(outlets []MonitorOutlet, ts time.Time) {
	if event, err := helper.OutletEvent(outlets, telemetry.Voltage, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Outlet Voltages")
	}

	if event, err := helper.OutletEvent(outlets, telemetry.Current, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Outlet Currents")
	}

	if event, err := helper.OutletEvent(outlets, telemetry.Energy, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Outlet Energy")
	}
}

func (helper *JAWsTelemetryHelper) PhaseEvent(phases []Phase, tType telemetry.CrayTelemetryType, timestamp time.Time) (telemetry.Event, error) {
	// Sensor identification:
	//	- Location is the PDU xname, such as x0m0p0
	//	- Physical context is always Phase
	//	- Sensor index is the id number of the Phase, Phase 2 has the index 2

	var sensors []telemetry.CraySensorPayload

	for _, phase := range phases {
		pduXName, err := helper.pduXName(phase.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		index, err := helper.parseIndex(phase.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		fromPhase, toPhase := getFromAndToPhasesFromPhaseName(phase.Name)

		var sensorValue string

		switch tType {
		case telemetry.Voltage:
			// Current is measured in Volts
			sensorValue = fmt.Sprintf("%f", phase.Voltage)
		case telemetry.Current:
			// Current is measured in Amps
			sensorValue = fmt.Sprintf("%f", phase.Current)
		case telemetry.Energy:
			// Energy is given as KiloWatt-Hours (kWh) from JAWS, Cray telemetry expects Joules
			// Conversion: 1 KiloWatt Hour == 3600 Joules
			energy := phase.Energy * 3600000
			sensorValue = fmt.Sprintf("%d", energy)
		default:
			return telemetry.Event{}, fmt.Errorf("Invalid telemetry type: %s", tType)
		}

		sensor := telemetry.CraySensorPayload{
			Timestamp:             timestamp.UTC().Format(time.RFC3339Nano),
			Location:              pduXName,
			Index:                 &index,
			Value:                 sensorValue,
			ParentalContext:       "PDU",
			PhysicalContext:       "Phase",
			PhysicalSubContext:    "Input", // Double Check that this is an input
			DeviceSpecificContext: "Line" + fromPhase + "ToLine" + toPhase,
		}

		sensors = append(sensors, sensor)
	}

	return helper.newEvent(timestamp, tType, sensors), nil
}

func (helper *JAWsTelemetryHelper) SendPhaseEvents(phases []Phase, ts time.Time) {
	if event, err := helper.PhaseEvent(phases, telemetry.Voltage, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Phase Voltages")
	}

	if event, err := helper.PhaseEvent(phases, telemetry.Current, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Phase Currents")
	}

	if event, err := helper.PhaseEvent(phases, telemetry.Energy, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Phase Energy")
	}
}

func (helper *JAWsTelemetryHelper) LineEvent(lines []Line, tType telemetry.CrayTelemetryType, timestamp time.Time) (telemetry.Event, error) {
	// Sensor identification:
	//	- Location is the PDU xname, such as x0m0p0
	//	- Physical context is always Line
	//	- Sensor index is the id number of the line, Line 2 has the index 2

	var sensors []telemetry.CraySensorPayload

	for _, line := range lines {
		pduXName, err := helper.pduXName(line.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		index, err := helper.parseIndex(line.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		// Strip the unit prefix to figure make the device context constent between 2 PDUs attached to the same PDU
		// cabinet controller. For example:
		// - Unit A: AA:L3 -> L3
		// - Unit B: BA:L3 -> L3
		deviceContext, err := helper.stripUnitPrefix(line.Name)
		if err != nil {
			return telemetry.Event{}, err
		}

		var sensorValue string

		switch tType {
		case telemetry.Current:
			// Current is measured in Amps
			sensorValue = fmt.Sprintf("%f", line.Current)
		default:
			return telemetry.Event{}, fmt.Errorf("Invalid telemetry type: %s", tType)
		}

		sensor := telemetry.CraySensorPayload{
			Timestamp:             timestamp.UTC().Format(time.RFC3339Nano),
			Location:              pduXName,
			Index:                 &index,
			Value:                 sensorValue,
			ParentalContext:       "PDU",
			PhysicalContext:       "Line",
			PhysicalSubContext:    "Input",
			DeviceSpecificContext: deviceContext,
		}

		sensors = append(sensors, sensor)
	}

	return helper.newEvent(timestamp, tType, sensors), nil
}

func (helper *JAWsTelemetryHelper) SendLineEvents(lines []Line, ts time.Time) {
	if event, err := helper.LineEvent(lines, telemetry.Current, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Line Currents")
	}
}

func (helper *JAWsTelemetryHelper) BranchEvent(branches []Branch, tType telemetry.CrayTelemetryType, timestamp time.Time) (telemetry.Event, error) {
	// Sensor identification:
	//	- Location is the PDU xname, such as x0m0p0
	//	- Physical context is always Branch
	//	- Sensor index is the id number of the branch, Branch 2 has the index 2

	var sensors []telemetry.CraySensorPayload

	for _, branch := range branches {
		pduXName, err := helper.pduXName(branch.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		index, err := helper.parseIndex(branch.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		// Strip the unit prefix to figure make the device context constent between 2 PDUs attached to the same PDU
		// cabinet controller. For example:
		// - Unit A: AA:L3-L1-BR6 -> L3-L1-BR6
		// - Unit B: BA:L3-L1-BR6 -> L3-L1-BR6
		deviceContext, err := helper.stripUnitPrefix(branch.Name)
		if err != nil {
			return telemetry.Event{}, err
		}

		var sensorValue string
		switch tType {
		case telemetry.Current:
			// Current is measured in Amps
			sensorValue = fmt.Sprintf("%f", branch.Current)
		default:
			return telemetry.Event{}, fmt.Errorf("Invalid telemetry type: %s", tType)
		}

		sensor := telemetry.CraySensorPayload{
			Timestamp:             timestamp.UTC().Format(time.RFC3339Nano),
			Location:              pduXName,
			Index:                 &index,
			Value:                 sensorValue,
			ParentalContext:       "PDU",
			PhysicalContext:       "Branch",
			PhysicalSubContext:    "Input",
			DeviceSpecificContext: deviceContext,
		}

		sensors = append(sensors, sensor)
	}

	return helper.newEvent(timestamp, telemetry.Current, sensors), nil
}

func (helper *JAWsTelemetryHelper) SendBranchEvents(branches []Branch, ts time.Time) {
	if event, err := helper.BranchEvent(branches, telemetry.Current, ts); err == nil {
		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Branch Currents")
	}
}

func (helper *JAWsTelemetryHelper) TemperatureEvent(temperatureSensors []TemperatureSensor, tType telemetry.CrayTelemetryType, timestamp time.Time) (telemetry.Event, error) {
	// Sensor identification:
	//	- Location is the PDU xname, such as x0m0p0
	//	- Physical context is always Temperature
	//	- Sensor index is the id number of the Temperature sensor, Temperature sensor 2 has the index 2

	var sensors []telemetry.CraySensorPayload

	for _, temperature := range temperatureSensors {
		pduXName, err := helper.pduXName(temperature.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		index, err := helper.parseIndex(temperature.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		var sensorValue string
		switch tType {
		case telemetry.Temperature:

			// PDUs report that they have 2 temperature sensors, even if they are not physically present.
			if temperature.TemperatureCelsius == nil {
				continue
			}

			// Temperature is measured in Celsius
			sensorValue = fmt.Sprintf("%f", *temperature.TemperatureCelsius)
		default:
			return telemetry.Event{}, fmt.Errorf("Invalid telemetry type: %s", tType)
		}

		sensor := telemetry.CraySensorPayload{
			Timestamp:             timestamp.UTC().Format(time.RFC3339Nano),
			Location:              pduXName,
			Index:                 &index,
			Value:                 sensorValue,
			ParentalContext:       "PDU",
			PhysicalContext:       "Temperature",
			DeviceSpecificContext: temperature.Name,
		}

		sensors = append(sensors, sensor)
	}

	return helper.newEvent(timestamp, telemetry.Temperature, sensors), nil
}

func (helper *JAWsTelemetryHelper) SendTemperatureEvents(sensors []TemperatureSensor, ts time.Time) {
	if event, err := helper.TemperatureEvent(sensors, telemetry.Temperature, ts); err == nil {
		if len(event.Events[0].Oem.Sensors) == 0 {
			// There are no temperature sensors attached to the PDU
			log.WithFields(log.Fields{
				"xname": helper.pduXName,
				"err":   err,
			}).Trace("Ignoring Temperature telemetry event, due to invalid temperature readings")
			return
		}

		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Temperature")
	}
}

func (helper *JAWsTelemetryHelper) HumidityEvent(humiditySensors []HumiditySensor, tType telemetry.CrayTelemetryType, timestamp time.Time) (telemetry.Event, error) {
	// Sensor identification:
	//	- Location is the PDU xname, such as x0m0p0
	//	- Physical context is always Humidity
	//	- Sensor index is the id number of the Temperature sensor, Temperature sensor 2 has the index 2

	var sensors []telemetry.CraySensorPayload

	for _, humidity := range humiditySensors {
		pduXName, err := helper.pduXName(humidity.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		index, err := helper.parseIndex(humidity.ID)
		if err != nil {
			return telemetry.Event{}, err
		}

		var sensorValue string
		switch tType {
		case telemetry.Temperature:

			// PDUs report that they have 2 humidity sensors, even if they are not physically present.
			if humidity.RelativeHumidity == nil {
				continue
			}

			// Humidity is measured in Relative Humidity %
			sensorValue = fmt.Sprintf("%f", *humidity.RelativeHumidity)
		default:
			return telemetry.Event{}, fmt.Errorf("Invalid telemetry type: %s", tType)
		}

		sensor := telemetry.CraySensorPayload{
			Timestamp:             timestamp.UTC().Format(time.RFC3339Nano),
			Location:              pduXName,
			Index:                 &index,
			Value:                 sensorValue,
			ParentalContext:       "PDU",
			PhysicalContext:       "Humidity", // Note: The Collector for river hardware puts FANs into the temperature telemetry kafka topic
			DeviceSpecificContext: humidity.Name,
		}

		sensors = append(sensors, sensor)
	}

	return helper.newEvent(timestamp, telemetry.Temperature, sensors), nil
}

func (helper *JAWsTelemetryHelper) SendHumidityEvents(sensors []HumiditySensor, ts time.Time) {
	if event, err := helper.HumidityEvent(sensors, telemetry.Temperature, ts); err == nil {
		if len(event.Events[0].Oem.Sensors) == 0 {
			// There are no temperature sensors attached to the PDU
			log.WithFields(log.Fields{
				"xname": helper.pduXName,
				"err":   err,
			}).Trace("Ignoring Humidity telemetry event, due to invalid humidity readings")
			return
		}

		helper.eventPayloads <- event
	} else {
		log.WithFields(log.Fields{
			"xname": helper.pduXName,
			"err":   err,
		}).Error("Unable to build telemetry event for Humidity")
	}
}
