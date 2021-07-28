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

package telemetry

import rf "github.com/Cray-HPE/hms-smd/pkg/redfish"

type CrayTelemetryType string

const (
	Voltage     CrayTelemetryType = "CrayTelemetry.Voltage"
	Current     CrayTelemetryType = "CrayTelemetry.Current"
	Energy      CrayTelemetryType = "CrayTelemetry.Energy"
	Temperature CrayTelemetryType = "CrayTelemetry.Temperature"
)

type CraySensorPayload struct {
	Timestamp             string
	Location              string
	ParentalContext       string `json:",omitempty"`
	ParentalIndex         *uint8 `json:",omitempty"`
	PhysicalContext       string
	Index                 *uint8 `json:",omitempty"`
	PhysicalSubContext    string `json:",omitempty"`
	DeviceSpecificContext string `json:",omitempty"`
	SubIndex              *uint8 `json:",omitempty"`
	Value                 string
}

type Sensors struct {
	Sensors         []CraySensorPayload
	TelemetrySource string
}

type EventRecord struct {
	EventType         string         `json:",omitempty"`
	EventId           string         `json:",omitempty"`
	EventTimestamp    string         `json:",omitempty"`
	Severity          string         `json:",omitempty"`
	Message           string         `json:",omitempty"`
	MessageId         string         `json:",omitempty"`
	MessageArgs       []string       `json:",omitempty"`
	Context           string         `json:",omitempty"` // Older versions
	OriginOfCondition *rf.ResourceID `json:",omitempty"`
	Oem               *Sensors       `json:",omitempty"` // Used only on for Cray RF events
}

type Event struct {
	OContext     string        `json:"@odata.context,omitempty"`
	Oid          string        `json:"@odata.id,omitempty"`
	Otype        string        `json:"@odata.type,omitempty"`
	Id           string        `json:"Id,omitempty"`
	Name         string        `json:"Name,omitempty"`
	Context      string        `json:"Context,omitempty"` // Later versions
	Description  string        `json:"Description,omitempty"`
	Events       []EventRecord `json:"Events,omitempty"`
	EventsOCount int           `json:"Events@odata.count,omitempty"`
}
