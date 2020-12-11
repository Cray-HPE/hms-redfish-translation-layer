/*
 * Shared Hook data structures
 *
 * Copyright 2018 Cray Inc.  All Rights Reserved
 *
 */
package hooks

import (
	"regexp"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfschema"
)

// Event is used to help notify hook callbacks of the current http operation in rdserver
type Event struct {
	HTTPOperation string
	URI           string
	Resource      *rfschema.Resource
	Property      *rfschema.Property
}

// HookIn is used to register callback functions for HTTP request for the rfserver
type HookIn struct {
	URIRegex *regexp.Regexp
	BeforeCB func(event Event, ctx, request map[string]interface{}) ([]byte, int, error)
	AfterCB  func(event Event, ctx, response map[string]interface{}) ([]byte, int, error)
}
