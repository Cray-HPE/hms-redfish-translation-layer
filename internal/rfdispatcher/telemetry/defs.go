// Copyright 2020 Hewlett Packard Enterprise Development LP

package telemetry

import rf "stash.us.cray.com/HMS/hms-smd/pkg/redfish"

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
