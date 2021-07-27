// MIT License
//
// (C) Copyright [2018, 2021] Hewlett Packard Enterprise Development LP
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

/*
 * Shared Hook data structures
 *
 */
package hooks

import (
	"regexp"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfschema"
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
