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
 * Json Schema File Service
 *
 */
package jsonschemas

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfdispatcher/hooks"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfschema"
	"strings"

	log "github.com/sirupsen/logrus"
)

// The JsonSchemaFileCollection represents a collection of JsonSchemaFiles.
// All of the members of this collection are auto generated from the schema.
type JsonSchemaFileCollection struct {
	schemaTree *rfschema.Tree
}

func NewJsonSchemaFileCollection(schemaTree *rfschema.Tree) *JsonSchemaFileCollection {
	js := &JsonSchemaFileCollection{}
	js.schemaTree = schemaTree
	return js
}

// Does the Schema file exist in this collection
func (js *JsonSchemaFileCollection) Exists(name string) bool {
	_, ok := js.schemaTree.SchemaPool.Pool[name]
	log.Trace(name, " exists in JSON schema file collection: ", ok)
	return ok
}

// A JsonSchemaFile describes where to go for the file describing the schema.
// This is a representation of the Redfish resource JsonSchemaFile
type JsonSchemaFile struct {
	ODataID      string `json:"@odata.id"`
	ODataType    string `json:"@odata.type"`
	ODataContext string `json:"@odata.context"`
	Languages    []string
	Location     []Location
	Schema       string
}

// Location describes the location of the
type Location struct {
	ArchiveFile    string `json:",omitempty"`
	ArchiveUri     string `json:",omitempty"`
	Language       string `json:",omitempty"`
	PublicationUri string `json:",omitempty"`
	Uri            string `json:",omitempty"`
}

// HandleCollection is a hook that will generate the "Members" property of a collection dynamically
// It is expected that the JsonSchema collection exists in Redis with no memebrs field
func (js *JsonSchemaFileCollection) HandleCollection(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	// TODO: In the future this cloud only return back schemas that are currently in use
	links := []string{}
	for name := range js.schemaTree.SchemaPool.Pool {
		link := "/redfish/v1/JSONSchemas/" + strings.TrimSuffix(name, ".json")
		links = append(links, link)
	}

	sort.Strings(links)

	members := []interface{}{}
	for _, link := range links {
		members = append(members, map[string]interface{}{"@odata.id": link})
	}

	response["Members"] = members
	response["Members@odata.count"] = len(members)

	return nil, 0, nil
}

// HandleResource is a hook that will dynamically generate a JsonSchemaFile resource.
// During a GET request it is a before hook that overrides the response payload with its own. Since this resource
// does not exist in redis.
func (js *JsonSchemaFileCollection) HandleResource(event hooks.Event, ctx, response map[string]interface{}) ([]byte, int, error) {
	// Determine with Json Schema File is requested
	tokens := strings.Split(event.URI, "/")
	id := tokens[len(tokens)-1]
	schemaFileName := id + ".json"
	schemaName := regexp.MustCompile(`^([A-Za-z0-9]+)`).FindString(id)
	log.Trace("Requested schema:", schemaName)

	schemaODataType := "#" + id + "." + schemaName
	schema, ok := js.schemaTree.SchemaPool.Pool[schemaFileName]
	if !ok {
		return nil, 0, errors.New("Json Schema does not exist: " + id)
	}

	file := JsonSchemaFile{}

	// OData generation
	file.ODataID = event.URI
	file.ODataType = event.Resource.ODataType
	fields := []string{"Languages", "Location", "Schema"}
	file.ODataContext = event.Resource.ODataContext + "(" + strings.Join(fields, ",") + ")"

	// Build JsonSchemaFile
	file.Schema = schemaODataType
	file.Languages = []string{"en-US"}
	location := Location{}
	location.Uri = "/redfish/v1/JsonSchemas/Files/" + schemaFileName

	// The $id field contains the URL of the DMTF resource
	if idRaw, ok := schema.Body["$id"]; ok {
		location.PublicationUri = idRaw.(string)
	}

	file.Location = []Location{location}

	// Build Response
	responseBody, err := json.Marshal(file)
	if err != nil {
		return nil, 0, err
	}

	return responseBody, http.StatusOK, errors.New("Overriding GET request")
}

// ServeSchemaFile is a http handler that will return the contents of a schema file
func (js *JsonSchemaFileCollection) ServeSchemaFile(w http.ResponseWriter, r *http.Request) {
	log.Debug("Servering schema file for: ", r.URL.Path)
	if !strings.HasPrefix(r.URL.Path, "/redfish/v1/JsonSchemas/Files/") {
		log.Warn("Requested schema file does not start with the correct path prefix")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	tokens := strings.Split(r.URL.Path, "/")
	schemaFileName := tokens[len(tokens)-1]
	log.Debug("Looking for schema file: ", schemaFileName)

	schema, ok := js.schemaTree.SchemaPool.Pool[schemaFileName]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Each rfschema.Schema contains an unmarshalled version of the schema
	responseBody, err := json.MarshalIndent(schema.Body, "", "  ")
	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseBody)
}
