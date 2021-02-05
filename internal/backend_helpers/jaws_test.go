// MIT License
//
// (C) Copyright [2020-2021] Hewlett Packard Enterprise Development LP
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
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/telemetry"
)

type JAWS_TS struct {
	suite.Suite
}

func (suite *JAWS_TS) Test_translateJAWSToRedfishOutlet() {
	input := []string{
		"BA1",
		"BA2",
		"BA3",
		"BA10",
		"BA11",
		"BA36",
	}
	expected := []string{
		"BA01",
		"BA02",
		"BA03",
		"BA10",
		"BA11",
		"BA36",
	}

	for i, outlet := range input {
		actual, err := translateJAWSToRedfishOutlet(outlet)
		suite.NoError(err)
		suite.Equal(expected[i], actual)
	}
}

func (suite *JAWS_TS) Test_translateRedfishToJAWSOutlet() {
	input := []string{
		"BA01",
		"BA02",
		"BA03",
		"BA10",
		"BA11",
		"BA36",
	}

	expected := []string{
		"BA1",
		"BA2",
		"BA3",
		"BA10",
		"BA11",
		"BA36",
	}

	for i, outlet := range input {
		actual, err := translateRedfishToJAWSOutlet(outlet)
		suite.NoError(err)
		suite.Equal(expected[i], actual)
	}
}

func (suite *JAWS_TS) Test_translateJAWSToRedfishOutlet_SadPath() {
	input := []string{
		"BA",
		"BAA",
		"",
	}

	for _, outlet := range input {
		actual, err := translateJAWSToRedfishOutlet(outlet)
		suite.Error(err)
		suite.Equal("", actual)
	}
}

func (suite *JAWS_TS) Test_translateRedfishToJAWSOutlet_SadPath() {
	input := []string{
		"BA",
		"BAA",
		"",
	}

	for _, outlet := range input {
		actual, err := translateRedfishToJAWSOutlet(outlet)
		suite.Error(err)
		suite.Equal("", actual)
	}
}

func Test_JAWS_BackendHelper(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)
	suite.Run(t, new(JAWS_TS))
}

type JAWSTelemetryHelper_TS struct {
	suite.Suite
	eventPayloads chan telemetry.Event
	tHelper       JAWsTelemetryHelper
}

func (suite *JAWSTelemetryHelper_TS) SetupTest() {
	suite.eventPayloads = make(chan telemetry.Event, 10000)
	suite.tHelper = JAWsTelemetryHelper{
		pduIDs:             []string{"A", "B"},
		pduControllerXName: "x3000m0",
		eventPayloads:      suite.eventPayloads,
	}
}

func (suite *JAWSTelemetryHelper_TS) Test_parseIndex() {
	inputIDs := []string{
		"BA36",
		"BA1",
		"BA01",
		"A1",
		"A2",
		"B1",
		"B2",
	}

	expectedIndex := []uint8{
		36,
		1,
		1,
		1,
		2,
		1,
		2,
	}

	for i, id := range inputIDs {
		index, err := suite.tHelper.parseIndex(id)
		suite.NoError(err, "Encountered error with id:", id)

		suite.Equal(expectedIndex[i], index)
	}
}

func (suite *JAWSTelemetryHelper_TS) Test_pduXName() {
	inputIDs := []string{
		"AA1",
		"AA01",
		"AA36",
		"BA1",
		"BA01",
		"BA36",
		"A1",
		"B2",
	}

	expectedXnames := []string{
		"x3000m0p0",
		"x3000m0p0",
		"x3000m0p0",
		"x3000m0p1",
		"x3000m0p1",
		"x3000m0p1",
		"x3000m0p0",
		"x3000m0p1",
	}

	for i, id := range inputIDs {
		xname, err := suite.tHelper.pduXName(id)
		suite.NoError(err, "Encountered error with id:", id)

		suite.Equal(expectedXnames[i], xname)
	}

}

func (suite *JAWSTelemetryHelper_TS) Test_stripUnitPrefix() {
	inputs := []string{
		"AA:L1",
		"AA:L2",
		"AA:L3",
		"BA:L1",
		"BA:L2",
		"BA:L3",
		"AA:L1-L2-BR1",
		"AA:L2-L3-BR2",
		"AA:L3-L1-BR3",
		"BA:L1-L2-BR1",
		"BA:L2-L3-BR2",
		"BA:L3-L1-BR3",
	}

	expected := []string{
		"L1",
		"L2",
		"L3",
		"L1",
		"L2",
		"L3",
		"L1-L2-BR1",
		"L2-L3-BR2",
		"L3-L1-BR3",
		"L1-L2-BR1",
		"L2-L3-BR2",
		"L3-L1-BR3",
	}

	for i, input := range inputs {
		value, err := suite.tHelper.stripUnitPrefix(input)
		suite.NoError(err, "Encountered error with input:", input)

		suite.Equal(expected[i], value)
	}
}

func (suite *JAWSTelemetryHelper_TS) Test_OutletEvent_Voltage() {
	outlets := []MonitorOutlet{
		MonitorOutlet{
			ID:      "BA24",
			Name:    "Link1_Outlet_24",
			Current: 0.18,
			Energy:  281423,
			Voltage: 207.7,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.OutletEvent(outlets, telemetry.Voltage, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Voltage")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p1")
	suite.Equal(*sensor.Index, uint8(24))
	suite.Equal(sensor.Value, "207.700000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Outlet")
	suite.Equal(sensor.PhysicalSubContext, "Output")
}

func (suite *JAWSTelemetryHelper_TS) Test_OutletEvent_Current() {
	outlets := []MonitorOutlet{
		MonitorOutlet{
			ID:      "BA24",
			Name:    "Link1_Outlet_24",
			Current: 0.18,
			Energy:  281423,
			Voltage: 207.7,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.OutletEvent(outlets, telemetry.Current, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Current")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p1")
	suite.Equal(*sensor.Index, uint8(24))
	suite.Equal(sensor.Value, "0.180000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Outlet")
	suite.Equal(sensor.PhysicalSubContext, "Output")
}

func (suite *JAWSTelemetryHelper_TS) Test_OutletEvent_Energy() {
	outlets := []MonitorOutlet{
		MonitorOutlet{
			ID:      "BA24",
			Name:    "Link1_Outlet_24",
			Current: 0.18,
			Energy:  281423,
			Voltage: 207.7,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.OutletEvent(outlets, telemetry.Energy, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Energy")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p1")
	suite.Equal(*sensor.Index, uint8(24))
	suite.Equal(sensor.Value, "1013122800")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Outlet")
	suite.Equal(sensor.PhysicalSubContext, "Output")
}

func (suite *JAWSTelemetryHelper_TS) Test_OutletEvent_InvalidTelemetryTypes() {
	outlets := []MonitorOutlet{
		MonitorOutlet{
			ID:      "BA24",
			Name:    "Link1_Outlet_24",
			Current: 0.18,
			Energy:  281423,
			Voltage: 207.7,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)

	_, err := suite.tHelper.OutletEvent(outlets, telemetry.Temperature, timestamp)
	suite.Error(err)
}

func (suite *JAWSTelemetryHelper_TS) Test_PhaseEvent_Voltage() {
	phases := []Phase{
		Phase{
			ID:      "AA1",
			Name:    "AA:L1-L2",
			Current: 1.62,
			Energy:  4177,
			Voltage: 208.1,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.PhaseEvent(phases, telemetry.Voltage, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Voltage")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(1))
	suite.Equal(sensor.Value, "208.100000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Phase")
	suite.Equal(sensor.PhysicalSubContext, "Input")
	suite.Equal(sensor.DeviceSpecificContext, "Line1ToLine2")
}

func (suite *JAWSTelemetryHelper_TS) Test_PhaseEvent_Current() {
	phases := []Phase{
		Phase{
			ID:      "AA1",
			Name:    "AA:L1-L2",
			Current: 1.62,
			Energy:  4177,
			Voltage: 208.1,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.PhaseEvent(phases, telemetry.Current, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Current")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(1))
	suite.Equal(sensor.Value, "1.620000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Phase")
	suite.Equal(sensor.PhysicalSubContext, "Input")
	suite.Equal(sensor.DeviceSpecificContext, "Line1ToLine2")
}

func (suite *JAWSTelemetryHelper_TS) Test_PhaseEvent_Energy() {
	phases := []Phase{
		Phase{
			ID:      "AA1",
			Name:    "AA:L1-L2",
			Current: 1.62,
			Energy:  4177,
			Voltage: 208.1,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.PhaseEvent(phases, telemetry.Energy, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Energy")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(1))
	suite.Equal(sensor.Value, "15037200000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Phase")
	suite.Equal(sensor.PhysicalSubContext, "Input")
	suite.Equal(sensor.DeviceSpecificContext, "Line1ToLine2")
}

func (suite *JAWSTelemetryHelper_TS) Test_PhaseEvent_InvalidTelemetryTypes() {
	phases := []Phase{
		Phase{
			ID:      "AA1",
			Name:    "AA:L1-L2",
			Current: 1.62,
			Energy:  4177,
			Voltage: 208.1,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)

	_, err := suite.tHelper.PhaseEvent(phases, telemetry.Temperature, timestamp)
	suite.Error(err)
}

func (suite *JAWSTelemetryHelper_TS) Test_LineEvent_Current() {
	lines := []Line{
		Line{
			ID:      "AA1",
			Name:    "AA:L1",
			Current: 2.80,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.LineEvent(lines, telemetry.Current, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Current")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(1))
	suite.Equal(sensor.Value, "2.800000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Line")
	suite.Equal(sensor.PhysicalSubContext, "Input")
	suite.Equal(sensor.DeviceSpecificContext, "L1")
}

func (suite *JAWSTelemetryHelper_TS) Test_LineEvent_InvalidTelemetryTypes() {
	lines := []Line{
		Line{
			ID:      "AA1",
			Name:    "AA:L1",
			Current: 2.80,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)

	invalidTypes := []telemetry.CrayTelemetryType{
		telemetry.Voltage,
		telemetry.Energy,
		telemetry.Temperature,
	}

	for _, invalidType := range invalidTypes {
		_, err := suite.tHelper.LineEvent(lines, invalidType, timestamp)
		suite.Error(err, "Expected error for telemetry type:", invalidType)
	}

}

func (suite *JAWSTelemetryHelper_TS) Test_BranchEvent_Current() {
	branches := []Branch{
		Branch{
			ID:      "AA1",
			Name:    "AA:L1-L2-BR1",
			Current: 2.80,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.BranchEvent(branches, telemetry.Current, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Current")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(1))
	suite.Equal(sensor.Value, "2.800000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Branch")
	suite.Equal(sensor.PhysicalSubContext, "Input")
	suite.Equal(sensor.DeviceSpecificContext, "L1-L2-BR1")
}

func (suite *JAWSTelemetryHelper_TS) Test_BranchEvent_InvalidTelemetryTypes() {
	branches := []Branch{
		Branch{
			ID:      "AA1",
			Name:    "AA:L1-L2-BR1",
			Current: 2.80,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)

	invalidTypes := []telemetry.CrayTelemetryType{
		telemetry.Voltage,
		telemetry.Energy,
		telemetry.Temperature,
	}

	for _, invalidType := range invalidTypes {
		_, err := suite.tHelper.BranchEvent(branches, invalidType, timestamp)
		suite.Error(err, "Expected error for telemetry type:", invalidType)
	}

}

func (suite *JAWSTelemetryHelper_TS) Test_TemperatureEvent_Temperature() {
	temperatureValue := 20.9
	sensors := []TemperatureSensor{
		TemperatureSensor{
			ID:                 "A2",
			Name:               "Temp_Sensor_A1",
			Status:             "Normal",
			TemperatureCelsius: &temperatureValue,
		},
		// No event record should be created for this temperature sensors, as it is not attached
		TemperatureSensor{
			ID:                 "B2",
			Name:               "Temp_Sensor_B2",
			Status:             "Not Found",
			TemperatureCelsius: nil,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.TemperatureEvent(sensors, telemetry.Temperature, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Temperature")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(2))
	suite.Equal(sensor.Value, "20.900000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Temperature")
	suite.Equal(sensor.DeviceSpecificContext, "Temp_Sensor_A1")
}

func (suite *JAWSTelemetryHelper_TS) Test_TemperatureEvent_InvalidTelemetryTypes() {
	temperatureValue := 20.9
	sensors := []TemperatureSensor{
		TemperatureSensor{
			ID:                 "A2",
			Name:               "Temp_Sensor_A1",
			Status:             "Normal",
			TemperatureCelsius: &temperatureValue,
		},
		// No event record should be created for this temperature sensor, as it is not attached
		TemperatureSensor{
			ID:                 "B2",
			Name:               "Temp_Sensor_B2",
			Status:             "Not Found",
			TemperatureCelsius: nil,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)

	invalidTypes := []telemetry.CrayTelemetryType{
		telemetry.Voltage,
		telemetry.Energy,
		telemetry.Current,
	}

	for _, invalidType := range invalidTypes {
		_, err := suite.tHelper.TemperatureEvent(sensors, invalidType, timestamp)
		suite.Error(err, "Expected error for telemetry type:", invalidType)
	}

}

func (suite *JAWSTelemetryHelper_TS) Test_HumidityEvent_Temperature() {
	humidityValue := 49.0
	sensors := []HumiditySensor{
		HumiditySensor{
			ID:               "A2",
			Name:             "Humid_Sensor_A1",
			Status:           "Normal",
			RelativeHumidity: &humidityValue,
		},
		// No event record should be created for this humidity sensor, as it is not attached
		HumiditySensor{
			ID:               "B2",
			Name:             "Humid_Sensor_B2",
			Status:           "Not Found",
			RelativeHumidity: nil,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)
	expectedTimeStamp := "2020-09-09T13:49:10.00001Z"

	event, err := suite.tHelper.HumidityEvent(sensors, telemetry.Temperature, timestamp)
	suite.NoError(err)
	suite.Equal(event.Context, "x3000m0:CrayTelemetry.Temperature")
	suite.Len(event.Events, 1)
	suite.Equal(event.EventsOCount, 1)

	eventRecord := event.Events[0]
	suite.Equal(eventRecord.EventTimestamp, expectedTimeStamp)
	suite.Equal(eventRecord.Message, "See Oem/CrayPayload property")
	suite.Empty(eventRecord.MessageArgs)
	suite.NotNil(eventRecord.Oem)

	oem := eventRecord.Oem
	suite.Equal(oem.TelemetrySource, "River")
	suite.Len(oem.Sensors, 1)

	sensor := oem.Sensors[0]
	suite.Equal(sensor.Timestamp, expectedTimeStamp)
	suite.Equal(sensor.Location, "x3000m0p0")
	suite.Equal(*sensor.Index, uint8(2))
	suite.Equal(sensor.Value, "49.000000")
	suite.Equal(sensor.ParentalContext, "PDU")
	suite.Equal(sensor.PhysicalContext, "Humidity")
	suite.Equal(sensor.DeviceSpecificContext, "Humid_Sensor_A1")
}

func (suite *JAWSTelemetryHelper_TS) Test_HumidityEvent_InvalidTelemetryTypes() {
	humidityValue := 49.0
	sensors := []HumiditySensor{
		HumiditySensor{
			ID:               "A2",
			Name:             "Humid_Sensor_A1",
			Status:           "Normal",
			RelativeHumidity: &humidityValue,
		},
		// No event record should be created for this humidity sensor, as it is not attached
		HumiditySensor{
			ID:               "B2",
			Name:             "Temp_Sensor_B2",
			Status:           "Not Found",
			RelativeHumidity: nil,
		},
	}

	timestamp := time.Date(2020, time.September, 9, 13, 49, 10, 10000, time.UTC)

	invalidTypes := []telemetry.CrayTelemetryType{
		telemetry.Voltage,
		telemetry.Energy,
		telemetry.Current,
	}

	for _, invalidType := range invalidTypes {
		_, err := suite.tHelper.HumidityEvent(sensors, invalidType, timestamp)
		suite.Error(err, "Expected error for telemetry type:", invalidType)
	}

}

func Test_JAWSTelemetryHelper(t *testing.T) {
	//This setups the production routs and handler
	logrus.SetLevel(logrus.TraceLevel)
	suite.Run(t, new(JAWSTelemetryHelper_TS))
}
