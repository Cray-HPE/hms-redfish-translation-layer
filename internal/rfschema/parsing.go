// MIT License
//
// (C) Copyright [2021] Hewlett Packard Enterprise Development LP
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

package rfschema

import (
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"path"
	"regexp"
	"strings"

	jptr "github.com/xeipuuv/gojsonpointer"
	jref "github.com/xeipuuv/gojsonreference"
)

// VersionResourceFileRegex is used to determine if a schema file is a versioned resource schema.
// The following regex will match
const VersionResourceFileRegex = `([A-Z][A-Za-z]+\.(v[0-9]*_[0-9]*_[0-9]*|test))\.json`

// Useful keys for common fields in the Redfish JSON schema
const (
	KeyRef              = "$ref"
	KeyDeprecated       = "deprecated"       // String
	KeyRequiredOnCreate = "requiredOnCreate" // Array of strings

	// Validation keywords for any type
	KeyType = "type" // String
	KeyEnum = "enum" // Array of any value. Though in the redfish schema this is always a string
	// KeyConst = "const"//

	// Validation keywords for numeric types
	KeyMaximum = "maximum" // Number
	KeyMinimum = "minimum" // Number
	KeyUnits   = "units"   // String

	// Validation keywords for strings
	KeyMaxLength = "maxLength" // Non-negative integer
	KeyMinLength = "minLength"
	KeyPattern   = "pattern" // String/Valid regular expression
	KeyFormat    = "format"

	// Validation keywords for arrays
	KeyItems = "items" // Schema

	// Validation keywords for objects
	KeyRequired             = "required"             // Array of strings
	KeyProperties           = "properties"           // Object/Map of JSON Schema?
	KeyPatternProperties    = "patternProperties"    // JSON schema?
	KeyAdditionalProperties = "additionalProperties" // Bool or Json Schema. RF uses only bool

	// Subschemas
	KeyAnyOf = "anyOf" // Array of JSON schemas

	// Schema reuse
	KeyDefinition = "definition" // Object/Map of JSON schemas

	// Schema Annotations
	KeyTitle           = "title"           // String
	KeyDescription     = "description"     // String
	KeyLongDescription = "longDescription" // String
	KeyReadOnly        = "readonly"        // bool
	KeyInsertable      = "insertable"      // bool
	KeyUpdatable       = "updatable"       // bool
	KeyDeletable       = "deletable"       // bool

	// Actions
	KeyParamaters        = "parameters"        // Schema
	KeyRequiredParameter = "requiredParameter" // Bool

	// Enums
	KeyEnumDescriptions     = "enumDescriptions"     // Map: enum name -> description
	KeyEnumLongDescriptions = "enumLongDescriptions" // Map: enum name -> description
	KeyEnumDeprecated       = "enumDeprecated"       // Map: enum name -> reason
)

// OemExtension contains the filename for OEM Extension schema and JSON reference to a defined schema.
// Example:
// {
//	"fileName": "TestOem.json",
//	"reference": "#/definitions/TestOem"
// }
type OemExtension struct {
	FileName  string `json:"fileName"`
	Reference string `json:"reference"`
}

// SchemaPool stores every schema contained in a schema directory. The pool's key is the schema file name.
// The OEM Extensions field contains all of the required mappings to correctly place OEM extension schemas into
// redfish resource when parsing. The key for OEM extensions follows this pattern: {Resource Name}/Path/To/Oem.
// For example if we wanted to attach a OEM extension schema to Chassis' Oem field the following key would be used:
// Chassis/Oem
// OemExtensions example:
// {
//     "Chassis/Oem": {
//         "fileName": "TestOem.json",
//         "reference": "#/definitions/TestOem"
//     }
// }
type SchemaPool struct {
	Pool          map[ /*Schema File Name*/ string]*Schema
	Version       Version
	OemExtensions map[string]OemExtension
}

// NewSchemaPool will initialize an empty pool
func NewSchemaPool(version Version) *SchemaPool {
	return &SchemaPool{
		Pool:    make(map[string]*Schema),
		Version: version,
	}
}

// Get the specified schema file from the pool
func (sp *SchemaPool) Get(schemaFile string) (*Schema, bool) {
	doc, ok := sp.Pool[schemaFile]
	return doc, ok
}

// ReadDir will read in all of the schema files present in the schema directory
func (sp *SchemaPool) ReadDir(schemaDir string) error {
	files, err := ioutil.ReadDir(schemaDir)
	if err != nil {
		return errors.New("Unable to read schema dir. Reason: " + err.Error())
	}

	for _, file := range files {
		filePath := path.Join(schemaDir, file.Name())
		schema, err := ReadSchema(filePath)
		if err != nil {
			return err
		}
		schema.ContainingPool = sp // Back pointer
		sp.Pool[file.Name()] = schema
	}

	return nil
}

// AddOemExtensions will take in a OEM mapping file and the directtory containing OEM extension schemas and add them
// to the schema pool. When resources are being parsed the OEM extension schemas will also be parsed.
// Warning: AddOemExtensions as currently implemented can only be called once on a Schema Pool instance, as pervious
// oem mappings will be overwritten.
// Example Oem Extension mapping file:
// {
//     "Chassis/Oem": {
//         "fileName": "TestOem.json",
//         "reference": "#/definitions/TestOem"
//     }
// }
func (sp *SchemaPool) AddOemExtensions(mappingFile string, extensionDir string) error {
	debugPrintln(0, "Adding oem extensions from:", mappingFile, "at", extensionDir)
	mappingRaw, err := ioutil.ReadFile(mappingFile)
	if err != nil {
		return errors.New("Unable to read oem mapping file. Reason: " + err.Error())
	}

	err = json.Unmarshal(mappingRaw, &sp.OemExtensions)
	if err != nil {
		return errors.New("Unable to parse oem mapping file. Reason: " + err.Error())
	}

	err = sp.ReadDir(extensionDir)
	if err != nil {
		return errors.New("Unable to read oem extension dir. Reason: " + err.Error())
	}

	return nil
}

// Schema represents a Redfish JSON schema document
type Schema struct {
	FileName string
	Body     map[string]interface{}
	TopLevel map[string]interface{} // The subschema linked to by the top level $ref elements

	AnyOf *Schema // Versions of this Schema

	Resource *Resource // Resource parsed by this schema

	ContainingPool *SchemaPool
}

// IsCollection will return true if this schema is a collection
func (s *Schema) IsCollection() bool {
	return strings.HasSuffix(s.FileName, "Collection.json")
}

// IsVersionedResource will return true if this schema is a versioned resource
func (s *Schema) IsVersionedResource() bool {
	ok, err := regexp.MatchString(VersionResourceFileRegex, s.FileName)
	if err != nil {
		panic(err)
	}
	return s.TopLevel != nil && ok
}

// IsResource will return true if this is a resource (including collections)
func (s *Schema) IsResource() bool {
	return s.TopLevel != nil && !s.IsVersionedResource()
}

func (s *Schema) followReference(pool *SchemaPool, refString string) (*jptr.JsonPointer, *Schema, map[string]interface{}) {
	// Is this reference a fragment for the current document?
	// Or is this a reference to another schema file?
	ref, err := jref.NewJsonReference(refString)
	if err != nil {
		panic(err)
	}

	if ref.HasFragmentOnly {
		// This reference is in the current document
		rawReferencedSchema, _, err := ref.GetPointer().Get(s.Body)
		if err != nil {
			log.WithFields(log.Fields{
				"FileName":  s.FileName,
				"refString": refString,
			}).Panic("File does not contain reference string")
		}
		return ref.GetPointer(), s, rawReferencedSchema.(map[string]interface{})
	}

	refURL := ref.GetUrl()
	debugPrintln(0, "URL", refURL.String())
	// This references another document
	if strings.HasPrefix(refURL.Path, "/schemas/v1/") {
		fileName := strings.TrimPrefix(refURL.Path, "/schemas/v1/")
		referencedSchema, ok := pool.Get(fileName)
		if !ok {
			panic("Referenced schema not found:" + refURL.String())
		}
		return referencedSchema.followReference(pool, "#"+ref.GetPointer().String())
	}
	return nil, nil, nil
}

// ReadSchema will read in the specifed schema file and then store it in a Schema structure
func ReadSchema(filePath string) (*Schema, error) {
	body, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, errors.New("Unable to read schema: " + filePath + " Reason: " + err.Error())

	}

	schema := &Schema{
		FileName: path.Base(filePath),
	}
	err = json.Unmarshal(body, &schema.Body)
	if err != nil {
		return nil, errors.New("Unable to unmarshal schema: " + filePath + " Reason: " + err.Error())

	}

	// Get the pointer to the "Top Level Schema"
	// In the Redfish schema Resources that are accessable from a URI contain a top level
	// $ref pointing to the Resources Schema.

	// TODO: Redundancy does not contain an top level references for some reason
	// For now lets manually extract
	//Hack: for some reason the "Redundancy" in the json schema is not a resource in the way the
	// other resources are represented in the schema. Even though it is a entity in the csdl
	// schema.
	var refString string
	refPresent := false
	if schema.FileName == "Redundancy.json" || regexp.MustCompile(`Redundancy\.v[0-9]_[0-9]_[0-9]\.json`).MatchString(schema.FileName) {
		refPresent = true
		refString = "#/definitions/Redundancy"
	} else {
		// This is valid for every other schema
		refPtr, err := jptr.NewJsonPointer("/" + KeyRef)
		if err != nil {
			panic(err)
		}
		rawRef, _, err := refPtr.Get(schema.Body)
		if err == nil {
			refString = rawRef.(string)
			refPresent = true
		}
	}
	if refPresent {
		ref, err := jref.NewJsonReference(refString)
		if err != nil {
			return nil, err
		}

		if ref.HasFragmentOnly {
			// If the reference is fragment, it means that the pointer is in the current document
			topLevelSchema, _, err := ref.GetPointer().Get(schema.Body)
			if err != nil {
				panic(err)
			}
			schema.TopLevel = topLevelSchema.(map[string]interface{})
		}
	}

	return schema, nil
}

// ParseCollection will parse any collection schema
// All collections have the same structure/properties, so a generic type that stores
// the member type will suffice
func ParseCollection(schema *Schema) (*Resource, string) {
	resource := &Resource{IsCollection: true}
	resource.Name = strings.TrimSuffix(path.Base(schema.FileName), ".json")

	// The collection schema can be access with the following JSON pointer from the top level
	// /anyOf/1
	ptr, err := jptr.NewJsonPointer("/anyOf/1")
	if err != nil {
		panic(err)
	}

	bodyRaw, _, err := ptr.Get(schema.TopLevel)
	if err != nil {
		panic(err)
	}
	body := bodyRaw.(map[string]interface{})

	// Extract OData Type and Context
	resource.ODataType = "#" + resource.Name + "." + resource.Name
	resource.ODataContext = "/redfish/v1/$metadata#" + resource.Name + "." + resource.Name

	resource.Description = extractDescription(body)
	resource.LongDescription = extractLongDescription(body)

	resource.Properties = findCollectionProperties(schema, body, []string{resource.Name})

	resource.RequiredProperties = findRequiredProperties(body)
	resource.RequiredOnCreate = findRequiredOnCreate(body)
	resource.AdditionalProperties = *findAdditionalProperties(body)
	resource.PatternProperties = findPatternProperties(body)

	// Extract Capabilities
	insertableRaw, ok := schema.TopLevel[KeyInsertable]
	if !ok {
		panic("Resource " + schema.FileName + " missing 'insertable' key")
	}
	resource.Insertable = insertableRaw.(bool)

	updatableRaw, ok := schema.TopLevel[KeyUpdatable]
	if !ok {
		panic("Resource " + schema.FileName + " missing 'updatable' key")
	}
	resource.Updatable = updatableRaw.(bool)

	deletableRaw, ok := schema.TopLevel[KeyDeletable]
	if !ok {
		panic("Resource " + schema.FileName + " missing 'deletable' key")
	}
	resource.Deletable = deletableRaw.(bool)

	return resource, resource.Name
}

// ParseResource will attempt to parse the provided Resource schema (for collections see ParseCollection).
func ParseResource(schema *Schema) (*Resource, string) {
	version2Ref := ExtractResourceVersions(schema.TopLevel) // Version -> $ref

	// // TODO: Redundancy does not contain an top level references for some reason
	// // For now lets manually extract
	// if schema.FileName == "Redundancy.json" {
	// 	ref, err := jref.NewJsonReference("#/definitions/Redundancy")
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	if ref.HasFragmentOnly {
	// 		// If the reference is fragment, it means that the pointer is in the current document
	// 		topLevelSchema, _, err := ref.GetPointer().Get(schema.Body)
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 		schema.TopLevel = topLevelSchema.(map[string]interface{})
	// 	}
	// }

	versions := []string{}
	for version := range version2Ref {
		versions = append(versions, version)
	}

	selectedVersion := selectResourceVersion(schema.FileName, schema.ContainingPool.Version, versions)
	refString := version2Ref[selectedVersion]

	resourceFileName := regexp.MustCompile(VersionResourceFileRegex).FindString(refString)
	if resourceFileName == "" {
		panic("Could not extract resource version: " + refString)
	}
	debugPrintln(0, "Using versioned resource:", resourceFileName)
	resourceSchema, ok := schema.ContainingPool.Get(resourceFileName)
	if !ok {
		panic("Schema not found: " + resourceFileName)
	}

	resource := parseVersionedResource(resourceSchema)

	// Extract OData Type and Context
	resource.ODataType = "#" + resource.Name + "." + resource.VersionString + "." + resource.Name
	resource.ODataContext = "/redfish/v1/$metadata#" + resource.Name + "." + resource.Name

	// Extract Capabilities
	// TODO: Remove the redundancy hacks
	if schema.FileName != "Redundancy.json" {
		insertableRaw, ok := schema.TopLevel[KeyInsertable]
		if !ok {
			panic("Resource " + schema.FileName + " missing 'insertable' key")
		}
		resource.Insertable = insertableRaw.(bool)

		updatableRaw, ok := schema.TopLevel[KeyUpdatable]
		if !ok {
			panic("Resource " + schema.FileName + " missing 'updatable' key")
		}
		resource.Updatable = updatableRaw.(bool)

		deletableRaw, ok := schema.TopLevel[KeyDeletable]
		if !ok {
			panic("Resource " + schema.FileName + " missing 'deletable' key")
		}
		resource.Deletable = deletableRaw.(bool)
	}

	return resource, resource.Name
}

// extractNameVersion will extact the name and version from the file name of some schema file
func extractNameVersion(resourceFileName string) (string, string) {
	// File format: ResourceName.Version.json
	// Ex: Chassis.v1_0_0.json
	nameVersion := strings.SplitN(resourceFileName, ".", 3)
	if len(nameVersion) != 3 {
		panic("Resource Name, Version or '.json' is missing")
	}

	return nameVersion[0], nameVersion[1]
}

// ParseEntity will parse any entity schema.
// An entity schema is used to specify fields for a resource that can be accessed by a URI.
//
func parseVersionedResource(schema *Schema) *Resource {
	resource := &Resource{}
	// name := strings.TrimSuffix(path.Base(schema.FilePath), ".json")

	name, version := extractNameVersion(schema.FileName)
	resource.Name = name
	resource.VersionString = version

	resource.Description = extractDescription(schema.TopLevel)
	resource.LongDescription = extractLongDescription(schema.TopLevel)

	resource.Properties = findResourceProperties(schema, []string{name})

	resource.RequiredProperties = findResourceRequiredProperties(schema)
	resource.RequiredOnCreate = findRequiredOnCreate(schema.TopLevel)
	resource.AdditionalProperties = *findAdditionalProperties(schema.TopLevel)
	resource.PatternProperties = findPatternProperties(schema.TopLevel)

	return resource
}

func findResourceProperties(schema *Schema, path []string) map[string]*Property {
	return schema.exploreObject(0, path, schema.TopLevel)
}

func findCollectionProperties(schema *Schema, body map[string]interface{}, path []string) map[string]*Property {
	return schema.exploreObject(0, path, body)
}

func findResourceRequiredProperties(schema *Schema) []string {
	if rawRequired, ok := schema.TopLevel[KeyRequired]; ok {
		var properties []string
		required := rawRequired.([]interface{})
		for _, rawProperty := range required {
			property := rawProperty.(string)
			properties = append(properties, property)
		}
		return properties
	}
	panic("Entity does not have a required property")
}

func findRequiredProperties(schema map[string]interface{}) []string {
	if rawRequired, ok := schema[KeyRequired]; ok {
		var properties []string
		required := rawRequired.([]interface{})
		for _, rawProperty := range required {
			property := rawProperty.(string)
			properties = append(properties, property)
		}
		return properties
	}
	panic("Entity does not have a required property")
}

func findAdditionalProperties(object map[string]interface{}) *bool {
	if rawAdditionalProps, ok := object[KeyAdditionalProperties]; ok {
		additionalProps := rawAdditionalProps.(bool)
		return &additionalProps
	}
	return nil
}

func findPatternProperties(object map[string]interface{}) []PatternProperties {
	if rawPatternProps, ok := object[KeyPatternProperties]; ok {
		patternProps := rawPatternProps.(map[string]interface{})

		var patterns []PatternProperties

		// There should only be one key in patternProperties
		for patternRegex, rawPatternObject := range patternProps {
			patternObject := rawPatternObject.(map[string]interface{})

			pattern := PatternProperties{
				Pattern: patternRegex,
			}

			// TODO: References in patterns are not current supported:
			// Such the following found in Resource.json#/definitions/Oem
			//  "patternProperties": {
			//     "[A-Za-z0-9_.:]+": {
			//         "$ref": "http://redfish.dmtf.org/schemas/v1/Resource.json#/definitions/OemObject"
			//     },

			// Get description
			rawDescription, ok := patternObject[KeyDescription]
			if ok {
				pattern.Description = rawDescription.(string)
			}

			// Get types for which this pattern applies to
			rawType, ok := patternObject[KeyType]
			if ok {
				switch rawType.(type) {
				case []interface{}:
					types := rawType.([]interface{})
					for _, rawType := range types {
						typeString := rawType.(string)
						pattern.Types = append(pattern.Types, typeString)
					}
				case string:
					typeString := rawType.(string)
					pattern.Types = append(pattern.Types, typeString)
				default:
					panic("Unsupported type")
				}
			}

			patterns = append(patterns, pattern)
		}

		return patterns
	}
	return nil
}

func findRequiredOnCreate(object map[string]interface{}) []string {
	var properties []string
	// Not every resource has a required on create property
	if rawRequired, ok := object[KeyRequiredOnCreate]; ok {
		required := rawRequired.([]interface{})
		for _, rawProperty := range required {
			property := rawProperty.(string)
			properties = append(properties, property)
		}
		return properties
	}
	return properties
}

func (s *Schema) exploreObject(level int, path []string, prop interface{}) map[string]*Property {
	properties := make(map[string]*Property)

	propPtr, err := jptr.NewJsonPointer("/properties")
	if err != nil {
		panic(err)
	}

	props, _, err := propPtr.Get(prop)
	if err != nil {
		panic(err)
	}

	for name, rawProp := range props.(map[string]interface{}) {
		prop := rawProp.(map[string]interface{})
		propPath := append(path, name)
		debugPrintln(level, name)

		var property *Property

		// If the name of this property is 'Actions'. This this property contains actions for this resource.
		if name == "Actions" {
			property = s.exploreActions(level+1, propPath, prop)
		} else if name == "Oem" {
			// Handle oem extensions
			property = handleOEMExtension(level+1, propPath, s.ContainingPool)
		} else if rawAnyOf, present := prop[KeyAnyOf]; present {
			// If $ref is present then this points to some other type
			// Otherwise
			// - anyOf is used to show the allowed types
			//   Most of the time it is null and a referenced type
			// - type is used is a string, or a array of strings for supported types
			// - readonly is a bool
			anyOf := rawAnyOf.([]interface{})
			debugPrintln(level+1, "- Any Of:")

			// There should only be 2 items in this any if array.
			// In the redfish schema the any of for properties is only contain a single reference and null
			if len(anyOf) != 2 {
				panic("Property " + name + " does not contain 2 elements in a AnyOf field")
			}

			// One element should be a reference, and the other should be null
			nullAllowed := false

			for _, rawElement := range anyOf {
				element := rawElement.(map[string]interface{})
				if rawRef, present := element[KeyRef]; present {
					ref := rawRef.(string)
					property = s.exploreRef(level+1, propPath, prop, ref)
				} else if rawType, present := element[KeyType]; present {
					typeStr := rawType.(string)
					if typeStr == "null" {
						nullAllowed = true
					} else {
						panic("Type other than null and $ref:" + typeStr)
					}
				}
			}

			property.NullAllowed = nullAllowed

		} else if rawRef, present := prop[KeyRef]; present {
			ref := rawRef.(string)
			property = s.exploreRef(level+1, propPath, prop, ref)
		} else if rawType, present := prop[KeyType]; present {
			property = s.exploreSchema(level+1, propPath, prop, rawType)
		}

		// If property is not nil, then add it
		if property != nil {
			// See if this property is deprecated
			if rawReason, ok := prop[KeyDeprecated]; ok {
				reason := rawReason.(string)
				property.Deprecated = true
				property.DeprecatedReason = &reason
			}

			// Is readonly present?
			if rawReadOnly, ok := prop[KeyReadOnly]; ok {
				readOnly := rawReadOnly.(bool)
				property.ReadOnly = readOnly
			}

			// Add the property
			properties[name] = property
		}

	}

	return properties
}

func (s *Schema) exploreType(level int, path []string, prop map[string]interface{}, rawType interface{}) *Property {
	var property *Property

	typeStr := rawType.(string)
	switch typeStr {
	case "object":
		// // TODO/BUG: When parsing say ComputerSystem->Links->CooledBy is not detected as a link, it is detected as a object.
		// // However links that are in arrays are properly handled. If http://redfish.dmtf.org/schemas/v1/odata.4.0.0.json#/definitions/idRef is used.
		// if _, present := prop["@odata.id"]; present && len(prop) == 1 {
		// 	// Its a Link!
		// 	property = &Property{
		// 		Type:    PropLink,
		// 		linkRef: "",
		// 		Link:    nil,
		// 	}
		// } else {
		// Its a object
		property = &Property{Type: PropObject}
		property.Properties = s.exploreObject(level, path, prop)

		// Extract additional properties and pattern properties
		property.AdditionalProperties = findAdditionalProperties(prop)
		property.PatternProperties = findPatternProperties(prop)
		// }

	case "array":
		property = &Property{Type: PropArray}
		elementProperty, refString := s.exploreArray(level, path, prop)
		if elementProperty == nil {
			return nil
		}
		property.ArrayOf = elementProperty
		property.arrayRefString = refString
	case "string":
		if rawEnums, ok := prop[KeyEnum]; ok {
			// This is a enum
			property = handleEnum(prop, rawEnums)
		} else {
			// This is just a normal string
			property = handleString(prop)
		}
	case "number":
		property = handleNumber(prop)
	case "integer":
		property = handleInteger(prop)
	case "boolean":
		property = handleBool(prop)
	default:
		panic("Unsupported type:" + typeStr + " in " + s.FileName)
	}

	property.Description = extractDescription(prop)
	property.LongDescription = extractLongDescription(prop)

	return property
}

func handleString(prop map[string]interface{}) *Property {
	property := &Property{Type: PropString}
	if rawMin, ok := prop[KeyMinLength]; ok {
		min := rawMin.(int)
		property.StringMinLength = &min
	}
	if rawMax, ok := prop[KeyMaxLength]; ok {
		max := rawMax.(int)
		property.StringMaxLength = &max
	}
	if rawPattern, ok := prop[KeyPattern]; ok {
		pattern := rawPattern.(string)
		property.StringPattern = &pattern
	}

	if rawFormat, ok := prop[KeyFormat]; ok {
		formatStr := rawFormat.(string)
		var format StringFormat
		switch formatStr {
		case "date-time":
			format = FormatDate
		case "uri":
			format = FormatURI
		}
		property.StringFormat = &format
	}
	return property
}

func handleEnum(prop map[string]interface{}, rawEnums interface{}) *Property {
	property := &Property{Type: PropEnum}
	enums := rawEnums.([]interface{})
	for _, rawEnum := range enums {
		enum := rawEnum.(string)
		enumMember := EnumMember{
			Name: enum,
		}
		property.EnumMembers = append(property.EnumMembers, enumMember)
	}
	// Deprecated Enums
	if rawDeprecatedEnums, ok := prop[KeyEnumDeprecated]; ok {
		deprecatedEnums := rawDeprecatedEnums.(map[string]interface{})
		property.DeprecatedEnums = make(map[string]string)
		for name, rawReason := range deprecatedEnums {
			reason := rawReason.(string)
			property.DeprecatedEnums[name] = reason
		}
	}

	// Enum Descriptions
	if rawEnumDescriptions, ok := prop[KeyEnumDescriptions]; ok {
		enumDescriptions := rawEnumDescriptions.(map[string]interface{})
		for i := range property.EnumMembers {
			enum := &property.EnumMembers[i]
			if rawDesc, ok := enumDescriptions[enum.Name]; ok {
				desc := rawDesc.(string)
				enum.Description = desc
			}
		}
	}
	if rawEnumLongDescriptions, ok := prop[KeyEnumLongDescriptions]; ok {
		enumLongDescriptions := rawEnumLongDescriptions.(map[string]interface{})
		for i := range property.EnumMembers {
			enum := &property.EnumMembers[i]
			if rawDesc, ok := enumLongDescriptions[enum.Name]; ok {
				longDesc := rawDesc.(string)
				enum.LongDescription = longDesc
			}
		}
	}

	return property
}

func handleNumber(prop map[string]interface{}) *Property {
	property := &Property{Type: PropNumber}
	if rawMin, ok := prop[KeyMinimum]; ok {
		min := rawMin.(float64)
		property.NumberMinimum = &min
	}
	if rawMax, ok := prop[KeyMaximum]; ok {
		max := rawMax.(float64)
		property.NumberMaximum = &max
	}
	if rawUnits, ok := prop[KeyUnits]; ok {
		units := rawUnits.(string)
		property.Units = &units
	}
	return property
}

func handleInteger(prop map[string]interface{}) *Property {
	property := &Property{Type: PropInteger}
	if rawMin, ok := prop[KeyMinimum]; ok {
		// json.Unmarshal places all numeric values into a float64
		min := int(rawMin.(float64))
		property.IntegerMinimun = &min
	}
	if rawMax, ok := prop[KeyMaximum]; ok {
		max := int(rawMax.(float64))
		property.IntegerMaximun = &max
	}
	if rawUnits, ok := prop[KeyUnits]; ok {
		units := rawUnits.(string)
		property.Units = &units
	}
	return property
}

func handleBool(map[string]interface{}) *Property {
	return &Property{Type: PropBool}
}

func handleOEMExtension(level int, path []string, sp *SchemaPool) *Property {
	oemKey := strings.Join(path, "/")
	if mapping, ok := sp.OemExtensions[oemKey]; ok {
		debugPrintln(level, "Found match!")
		schema, present := sp.Get(mapping.FileName)
		if !present {
			panic("Oem extension schema not present: " + mapping.FileName)
		}
		return schema.exploreRef(level, path, nil, mapping.Reference)
	}

	// Return nothing
	// TODO: Get schema provided descriptions in this case
	return &Property{Type: PropObject}
}

func (s *Schema) exploreSchema(level int, path []string, prop map[string]interface{}, rawType interface{}) *Property {
	switch rawType.(type) {
	case string:
		return s.exploreType(level, path, prop, rawType)
	case []interface{}:
		// An array of types must be a array of unique strings.
		// Each string must be one of the following primitive types: "null", "boolean", "object", "array", "number", "string", or "integer"

		// Most instances when multiple types are specified is when in a property that has the types of null, and some primitive type.
		typeArray := rawType.([]interface{})

		if len(typeArray) == 2 && typeArray[0] == "null" {
			property := s.exploreType(level, path, prop, typeArray[1])
			if property != nil {
				property.NullAllowed = true
			}
			return property
		} else if len(typeArray) == 2 && typeArray[1] == "null" {
			property := s.exploreType(level, path, prop, typeArray[0])
			if property != nil {
				property.NullAllowed = true
			}
			return property
		} else {
			// TODO document where in the 2018.1 schema that this occurs
			combinedProperty := &Property{Type: PropMultiple}
			combinedProperty.Description = extractDescription(prop)
			combinedProperty.LongDescription = extractLongDescription(prop)

			for _, rawType := range typeArray {
				typeString := rawType.(string)
				if typeString == "null" {
					combinedProperty.NullAllowed = true
				} else {
					property := s.exploreType(level+1, path, prop, typeString)
					if property == nil {
						// Exploring this type failed, return a nil pointer
						return nil
					}
					if property.NullAllowed {
						combinedProperty.NullAllowed = true
					}
					switch property.Type {
					case PropObject:
						combinedProperty.Properties = property.Properties
						combinedProperty.AdditionalProperties = property.AdditionalProperties
						combinedProperty.PatternProperties = property.PatternProperties
					case PropAction:
						combinedProperty.Parameters = property.Parameters
						combinedProperty.RequiredParameters = property.RequiredParameters
						combinedProperty.Target = property.Target
						combinedProperty.Title = property.Title
					case PropEnum:
						combinedProperty.EnumMembers = property.EnumMembers
					case PropString:
						combinedProperty.StringFormat = property.StringFormat
						combinedProperty.StringMaxLength = property.StringMaxLength
						combinedProperty.StringMinLength = property.StringMinLength
						combinedProperty.StringPattern = property.StringPattern
					case PropNumber:
						combinedProperty.NumberMaximum = property.NumberMaximum
						combinedProperty.NumberMinimum = property.NumberMinimum
						combinedProperty.Units = property.Units
					case PropBool:
						// Nothing to copy over
					case PropArray:
						combinedProperty.ArrayOf = property.ArrayOf
						combinedProperty.arrayRefString = property.arrayRefString
					case PropLink:
						// A link are not an primitive type
						panic("A link is not a primitive type")
					case PropMultiple:
						// No primitive type has multiple types
						panic("A multiple data type is not a primitive type")
					default:
						panic("Unsupported type:" + property.Type.String())
					}

					combinedProperty.Types = append(combinedProperty.Types, property.Type)
				}
			}
			return combinedProperty
		}
	default:
		// A type field must be either a string or a array of unique strings
		// http://json-schema.org/latest/json-schema-validation.html#rfc.section.6.1.1
		panic("Invalid type for 'type' field")
	}
}

func (s *Schema) exploreArray(level int, path []string, prop map[string]interface{}) (*Property, *string) {
	// The following is valid for DSP8010 Version 2018.1 (and below?)
	// In the redfish schema there are not Arrays do not contain any array of items. Only a single item.
	//
	// Array items can be a reference to someother schema
	//
	// Array items in the redfish schema either have a single type specified as a string, or two
	// types specified as a array of strings with the length 2 with a primitive type, and null.
	// This string is one of the primitive types: "boolean", "object", "array", "number", "string",
	// excluding null.
	//
	// In some cases the items field contains a anyOf schema which usually means that the item
	// consists of reference and null.

	// TODO: How should path be handled?
	// Should something be appended to the path?

	if rawItem, ok := prop[KeyItems]; ok {
		debugPrintln(level, "- Array Of:")
		switch rawItem.(type) {
		case []interface{}:
			panic("Cannot handle arrays of multiple types")
		case map[string]interface{}:
			item := rawItem.(map[string]interface{})

			if rawRef, present := item[KeyRef]; present {
				refString := rawRef.(string)
				return s.exploreRef(level+1, path, nil, refString), &refString
			} else if rawItemType, present := item[KeyType]; present {
				switch rawItemType.(type) {
				case []interface{}:
					// All elements
					itemTypes := rawItemType.([]interface{})
					if len(itemTypes) != 2 {
						panic("Item array contains more than 2 type elements")
					}

					typeStr0 := itemTypes[0].(string)
					typeStr1 := itemTypes[1].(string)
					var property *Property

					if typeStr0 == "null" {
						property = s.exploreType(level, path, prop, typeStr1)
					} else if typeStr1 == "null" {
						property = s.exploreType(level, path, prop, typeStr0)
					} else {
						panic("Neither of the 2 types were null")
					}

					if property != nil {
						property.NullAllowed = true
						return property, nil
					}
					return nil, nil
				case interface{}:
					// Single item type
					// Note: this needs to be after []interface{}
					return s.exploreType(level+1, path, prop, rawItemType), nil
				default:
					panic("Cannot handle type")
				}
			} else if rawAnyOf, present := item[KeyAnyOf]; present {
				// Assumption: If anyOf is used in a array then it contains 2 elements, null and a reference / primitive type
				// TODO support when null is not present
				// TODO can this be merged with the other instance of anyOf handling?
				anyOf := rawAnyOf.([]interface{})
				if len(anyOf) != 2 {
					panic("Item array contains more than 2 type elements")
				}

				anyOf0 := anyOf[0].(map[string]interface{})
				anyOf1 := anyOf[1].(map[string]interface{})
				var property *Property

				if rawRef, refOk := anyOf0[KeyRef]; refOk && anyOf1[KeyType] == "null" {
					property = s.exploreRef(level, path, nil, rawRef.(string))
				} else if rawRef, refOk := anyOf1[KeyRef]; refOk && anyOf0[KeyType] == "null" {
					property = s.exploreRef(level, path, nil, rawRef.(string))
				} else {
					// Our assumption is invalid
					panic("Could not handle anyOf")
				}

				if property != nil {
					property.NullAllowed = true
					return property, nil
				}
				return nil, nil

			}
			// The array contains something that is unexpected
			panic("Could not handle array")
		default:
			// A item entry must be either a schema (map[string]interace{}) or a array of schemas
			panic("Unknown type")
		}
	}
	// This is not a legitmate array
	panic("Does not have items key")
}

func (s *Schema) exploreRef(level int, path []string, referingObject map[string]interface{}, refString string) *Property {
	// Types of references
	// - Embed the reference into the document
	// 	- Such as new references to Status, Health, Links
	// - Link to another resource
	//	- What generated "@odata.id" links in responses
	debugPrintln(level, "- Reference to", refString)

	// Ignore any reference to SwordFish schemas. For our purposes this can be safely ignored
	if strings.Contains(refString, "swordfish") {
		return nil
	}

	if strings.Contains(refString, "#/definitions/idRef") {
		property := &Property{
			Type: PropLink,
			// linkRef: refString,
		}
		property.Description = extractDescription(referingObject)
		return property
	}

	refPtr, refSchema, reference := s.followReference(s.ContainingPool, refString)
	if reference != nil {
		if isResourceLink(refPtr, refSchema, s) {
			property := &Property{
				Type:    PropLink,
				linkRef: refString,
			}
			property.Description = extractDescription(referingObject)
			return property
		} else if rawType, present := reference[KeyType]; present {
			debugPrintln(level+1, "(exploring reference", refString, ")")
			return refSchema.exploreSchema(level+1, path, reference, rawType)
		} else if _, ok := reference[KeyRef]; ok {
			panic("What?") //remove?
		} else if _, ok := reference[KeyAnyOf]; ok {
			// If anyof is present this this is a versioned object
			return refSchema.exploreVersionedObject(level+1, path, refPtr, refSchema)
		} else if len(reference) == 0 {
			// The referenced schema contains nothing, which by the JSON Schema it would be the schema '{}'
			// Which means everything is a valid.
			// As of schema version 2018.1 this only occurs when http://redfish.dmtf.org/schemas/v1/Resource.json#/definitions/Resource
			// is referenced. Which in turn points to http://redfish.dmtf.org/schemas/v1/Resource.v1_0_0.json#/definitions/Resource"
			// which is the empty schema

			// TODO For now leave this as a unknown property
			if refString == "http://redfish.dmtf.org/schemas/v1/Resource.v1_0_0.json#/definitions/Resource" {
				return &Property{Type: PropLink, linkRef: refString}
			}
			return &Property{Type: PropUnknown}
		}
		panic("Unable to handle reference")
	}

	// The desired reference points to a nonexistant schema
	panic("Reference not present:" + refString)
}

func isResourceLink(refPtr *jptr.JsonPointer, refSchema, srcSchema *Schema) bool {
	// For reference to be considered a link, it needs to contain an anyOf validation field.
	// If this field contains a idRef then that resource can be linked to.

	// For example: http://redfish.dmtf.org/schemas/v1/Resource.json#/definitions/UUID
	// Would not be a link since it lacks a IdRef in its anyOf found in Resource.json

	// For example: "$ref": "http://redfish.dmtf.org/schemas/v1/ComputerSystemCollection.json#/definitions/ComputerSystemCollection",
	// Would be a link since it does contain a IdRef in its anyOf found in ComputerSystem.json

	linkRefFragment := "#" + refPtr.String()

	// TODO Move into mapping json file perhaps?
	type LinkException struct {
		From    string
		To      string
		LinkRef string
	}
	exceptions := []LinkException{
		{
			From:    "Storage.v1_4_0.json",
			To:      "Storage.json",
			LinkRef: "#/definitions/StorageController",
		},
	}

	isLinkException := func(sourceFileName, resourceName, linkRef string) bool {
		for _, exception := range exceptions {
			if exception.From == sourceFileName && exception.To == resourceName && exception.LinkRef == linkRef {
				return true
			}
		}

		return false
	}

	if isLinkException(srcSchema.FileName, refSchema.FileName, linkRefFragment) {
		return false
	}

	// Check AnyOf Field
	rawRefBody, _, err := refPtr.Get(refSchema.Body)
	if err != nil {
		panic(err)
	}

	refBody := rawRefBody.(map[string]interface{})

	if rawAnyOf, ok := refBody[KeyAnyOf]; ok {
		anyOf := rawAnyOf.([]interface{})
		for _, rawElement := range anyOf {
			element := rawElement.(map[string]interface{})
			if rawRef, ok := element[KeyRef]; ok {
				ref := rawRef.(string)
				if strings.Contains(ref, "#/definitions/idRef") {
					return true
				} else if strings.Contains(ref, "#/definitions/Resource") {
					return true
				}
			}
		}
	}

	return false
}

func (s *Schema) exploreVersionedObject(level int, path []string, refPtr *jptr.JsonPointer, refSchema *Schema) *Property {
	// A versioned object is object that is referenced in the redfish schema has multiple version to it.
	// One example of this in the redfish schema are schemas defined in IPAddresses.json
	// Such as:
	// "IPv4Address": {
	// 	"anyOf": [
	// 		{ "$ref": "http://redfish.dmtf.org/schemas/v1/IPAddresses.v1_0_0.json#/definitions/IPv4Address" },
	// 		{ "$ref": "http://redfish.dmtf.org/schemas/v1/IPAddresses.v1_0_2.json#/definitions/IPv4Address" }
	// 	],
	// }
	// Every entry in the anyOf should be a reference

	rawObject, _, err := refPtr.Get(refSchema.Body)
	if err != nil {
		panic(err)
	}
	object := rawObject.(map[string]interface{})
	version2Ref := ExtractResourceVersions(object) // Version -> $ref

	versions := []string{}
	for version := range version2Ref {
		versions = append(versions, version)
	}

	selectedVersion := selectObjectVersion(refSchema.FileName, s.ContainingPool.Version, versions)
	refProperty := refSchema.exploreRef(level, path, nil, version2Ref[selectedVersion])
	return refProperty
}

func (s *Schema) exploreActions(level int, path []string, actions map[string]interface{}) *Property {
	// Actions in a Redfish schema should be contained in the resource's 'Actions' property.
	// Here actions is the contents of that property.
	// The actions property will either contain a with properties defining the available actions,
	// or a reference to this object.
	if rawRef, ok := actions[KeyRef]; ok {
		refString := rawRef.(string)

		_, refSchema, reference := s.followReference(s.ContainingPool, refString)
		if reference != nil {
			return refSchema.exploreActions(level+1, path, reference)
		}
		panic("Reference could not be followed: " + refString)
	} else {
		property := &Property{
			Type:       PropObject,
			Properties: make(map[string]*Property),
		}

		// Extract Descriptions
		property.Description = extractDescription(actions)
		property.LongDescription = extractLongDescription(actions)

		// Extract additional properties and pattern properties
		property.AdditionalProperties = findAdditionalProperties(actions)
		property.PatternProperties = findPatternProperties(actions)

		rawProperties, ok := actions[KeyProperties]
		if !ok {
			panic("Actions does not contain a properties property")
		}

		properties := rawProperties.(map[string]interface{})
		for name, rawActionProperties := range properties {
			actionProperties := rawActionProperties.(map[string]interface{})
			action := s.exploreAction(level+1, append(path, name), actionProperties)

			if action == nil {
				panic("Action is nil")
			}

			// if action != nil {
			property.Properties[name] = action
			// }
		}

		return property
	}
}

func (s *Schema) exploreAction(level int, path []string, actionFields map[string]interface{}) *Property {
	// A Action will either contain an object with the needed parameters, or a references to the action
	if rawRef, ok := actionFields[KeyRef]; ok {
		refString := rawRef.(string)

		_, refSchema, reference := s.followReference(s.ContainingPool, refString)
		if reference != nil {
			return refSchema.exploreAction(level+1, path, reference)
		}
		panic("Reference could not be followed: " + refString)
	} else {
		action := Property{
			Type:       PropAction,
			Parameters: make(map[string]*Property),
		}
		action.Description = extractDescription(actionFields)
		action.LongDescription = extractLongDescription(actionFields)

		if rawParamaters, ok := actionFields[KeyParamaters]; ok {
			// Note some actions such as OemActions do not contain a parameter field,
			// in these cases ignore the paramaters.
			for name, rawParam := range rawParamaters.(map[string]interface{}) {
				param := rawParam.(map[string]interface{})
				var property *Property
				if rawRef, present := param[KeyRef]; present {
					ref := rawRef.(string)
					property = s.exploreRef(level+1, path, actionFields, ref)
				} else if rawType, present := param[KeyType]; present {
					property = s.exploreSchema(level+1, path, param, rawType)

				} else {
					panic("Can't handle action")
				}

				if property != nil {
					// Is this paramater required?
					if rawRequired, ok := param[KeyRequiredParameter]; ok {
						required := rawRequired.(bool)
						if required {
							action.RequiredParameters = append(action.RequiredParameters, name)
						}
					}

					// Is readonly present?
					if rawReadOnly, ok := param[KeyReadOnly]; ok {
						readOnly := rawReadOnly.(bool)
						property.ReadOnly = readOnly
					}
					action.Parameters[name] = property
				}

			}
		}

		if _, ok := actionFields[KeyProperties]; ok {
			// Extract the actions Target and Title
			properties := s.exploreObject(level+1, path, actionFields)
			action.Target = properties["target"]
			action.Title = properties["title"]
		}

		return &action
	}
}

func extractDescription(obj map[string]interface{}) *string {
	if rawDesc, ok := obj[KeyDescription]; ok {
		desc := rawDesc.(string)
		return &desc
	}
	return nil
}

func extractLongDescription(obj map[string]interface{}) *string {
	if rawLongDesc, ok := obj[KeyLongDescription]; ok {
		longDesc := rawLongDesc.(string)
		return &longDesc
	}
	return nil
}
