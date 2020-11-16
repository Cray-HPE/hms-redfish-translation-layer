/*
 * Validate Redfish payloads against the Redfish Schema
 *
 * Copyright 2018 Cray Inc.  All Rights Reserved
 *
 */
package main

import (
	"fmt"
	"regexp"
	"sort"
	"stash.us.cray.com/HMS/hms-redfish-translation-service/internal/rfschema"

	log "github.com/sirupsen/logrus"
)

type ObjectReport struct {
	// Object
	Missing                 map[string]bool
	Added                   map[string]bool
	InvalidNames            map[string]bool
	MissingRequired         map[string]bool
	MissingRequiredOnCreate map[string]bool

	// Enum
	InvalidEnum map[string]bool // Enum property contains invalid value

	// String
	InvalidString map[string]bool

	// Number
	InvalidNumber  map[string]bool
	InvalidInteger map[string]bool // TODO validate

	InvalidBool   map[string]bool
	InvalidLink   map[string]bool
	InvalidObject map[string]bool
	InvalidArray  map[string]bool

	Properties map[string]ObjectReport
	Arrays     map[string]ArrayReport
}

func (or *ObjectReport) getMissingRequired() []string {
	missing := []string{}
	for name, ok := range or.MissingRequired {
		if ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func (or *ObjectReport) getMissingRequiredOnCreate() []string {
	missing := []string{}
	for name := range or.MissingRequiredOnCreate {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing
}

func logSet(set map[string]bool, message, key string) {
	names := []string{}
	for name, ok := range set {
		if ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		log.Debug(message, key+"/"+name)
	}
}

func (or *ObjectReport) log(key string) {
	// Missing required keys
	logSet(or.MissingRequired, "Missing required property", key)

	// Missing required on create keys
	logSet(or.MissingRequiredOnCreate, "Missing required property on create", key)

	// Invalid names
	logSet(or.InvalidNames, "Invalid Property name", key)

	// Invalid Numbers
	logSet(or.InvalidNumber, "Invalid number value", key)

	// Invalid integers
	logSet(or.InvalidInteger, "Invalid integer value", key)

	// Invalid Enums
	logSet(or.InvalidEnum, "Invalid enum value", key)

	// Invalid Strings
	logSet(or.InvalidString, "Invalid string value", key)

	// Invalid Bools
	logSet(or.InvalidBool, "Invalid bool value", key)

	// Invalid Objects
	logSet(or.InvalidObject, "Invalid object value", key)

	// Invalid Arrays
	logSet(or.InvalidArray, "Invalid array value", key)

	// Invalid Links
	logSet(or.InvalidLink, "Invalid link value", key)

	for name, pReport := range or.Properties {
		pReport.log(key + "/" + name)
	}

	for name, aReport := range or.Arrays {
		aReport.log(key + "/" + name)
	}
}

func NewObjectReport() ObjectReport {
	return ObjectReport{
		Missing:                 map[string]bool{},
		Added:                   map[string]bool{},
		InvalidNames:            map[string]bool{},
		MissingRequired:         map[string]bool{},
		MissingRequiredOnCreate: map[string]bool{},
		InvalidNumber:           map[string]bool{},
		InvalidEnum:             map[string]bool{},
		InvalidString:           map[string]bool{},
		InvalidBool:             map[string]bool{},
		InvalidObject:           map[string]bool{},
		InvalidArray:            map[string]bool{},
		InvalidLink:             map[string]bool{},
		Properties:              map[string]ObjectReport{},
		Arrays:                  map[string]ArrayReport{},
	}
}

type ArrayReport struct {
	InvalidElements map[int]bool

	Properties map[int]ObjectReport
}

func (ar *ArrayReport) log(key string) {
	for index, invalid := range ar.InvalidElements {
		if invalid {
			log.Debug("Invalid element value", key+"/"+fmt.Sprintf("%d", index))
		}
	}
	for index, pReport := range ar.Properties {
		pReport.log(key + "/" + fmt.Sprintf("%d", index))
	}
}

func NewArrayReport() ArrayReport {
	return ArrayReport{
		InvalidElements: map[int]bool{},
		Properties:      map[int]ObjectReport{},
	}
}

// Validate the resource object
func checkResource(resource *rfschema.Resource, object map[string]interface{}) ObjectReport {
	// Check ODataType
	// TODO

	// Check properties
	// Find missing & added properties
	objectReport := checkProperties(object, resource.Properties, resource.AdditionalProperties, resource.PatternProperties)

	// Check for required properties
	for _, name := range resource.RequiredProperties {
		if _, ok := object[name]; !ok {
			objectReport.MissingRequired[name] = true
			// fmt.Println("Missing required property:", name)
		}
	}

	// Check for required on create properties
	for _, name := range resource.RequiredOnCreate {
		if _, ok := object[name]; !ok {
			objectReport.MissingRequiredOnCreate[name] = true
			// fmt.Println("Missing required property:", name)
		}
	}

	return objectReport
}

func checkObject(payload map[string]interface{}, schemaObject *rfschema.Property) ObjectReport {
	// Objects do not have required properties
	additionalProperties := false // Defaults to false
	if schemaObject.AdditionalProperties != nil {
		additionalProperties = *schemaObject.AdditionalProperties
	}
	return checkProperties(payload, schemaObject.Properties, additionalProperties, schemaObject.PatternProperties)

}

func checkProperties(payload map[string]interface{}, properties map[string]*rfschema.Property, additionalProperties bool, patternProperties []rfschema.PatternProperties) ObjectReport {
	op := NewObjectReport()
	// Find missing & added properties
	for name := range payload {
		if _, ok := properties[name]; !ok {
			// This property has been added
			op.Added[name] = true
			// fmt.Println("Added property:", name)
			if !additionalProperties {
				// Check to see if name comforms to on of the pattern properties
				for _, pattern := range patternProperties {
					if !pattern.Regex().MatchString(name) {
						op.InvalidNames[name] = true
						break
					}
				}
			}
		}
	}

	for name, property := range properties {
		payloadRaw, ok := payload[name]
		if !ok {
			// This property has been removed
			op.Missing[name] = true
			// fmt.Println("Missing property:", name)
			continue
		}

		// Check for type correctness
		// TODO: Check to see if the value type is acceptable
		// TODO: Check to see if the returned value matches redis
		// Check for type correctness
		// TODO: refactor the following into check property

		switch property.Type {
		case rfschema.PropObject:
			payloadProperty, ok := payloadRaw.(map[string]interface{})
			if ok {
				op.Properties[name] = checkObject(payloadProperty, property)
			} else {
				op.InvalidObject[name] = true
			}
		case rfschema.PropAction:
			checkAction()
		case rfschema.PropEnum:
			op.InvalidEnum[name] = !checkEnum(payloadRaw, property)
		case rfschema.PropString:
			op.InvalidString[name] = !checkString(payloadRaw, property)
		case rfschema.PropInteger:
			fallthrough
		case rfschema.PropNumber:
			op.InvalidNumber[name] = !checkNumber(payloadRaw, property)
		case rfschema.PropBool:
			op.InvalidBool[name] = !checkBool(payloadRaw, property)
		case rfschema.PropArray:
			payloadArray, ok := payloadRaw.([]interface{})
			if ok {
				op.Arrays[name] = checkArray(payloadArray, property)
			} else {
				op.InvalidArray[name] = true
			}
		case rfschema.PropLink:
			if property.IsNavLink && property.LinkAutoExpand {
				// This collection is allowed to auto expanded
				// Since the collection is embedded it can be treated as an array
				// (since that is how it is represented in the JSON payload)
				payloadArray, ok := payloadRaw.([]interface{})
				if ok {
					op.Arrays[name] = checkArray(payloadArray, property)
				} else {
					op.InvalidArray[name] = true
				}
			} else {
				op.InvalidLink[name] = !checkLink(payloadRaw, property)
			}
		default:
			panic("Cannot handle type:" + property.Type.String())
		}
	}
	return op
}

func checkAction() {
	// TODO
}

// validateEnum will return true if the enum contains one of the allowed
// values
func checkEnum(payloadEnumRaw interface{}, property *rfschema.Property) bool {
	payloadEnum, ok := payloadEnumRaw.(string)
	// Check type correctness
	if !ok {
		return false
	}
	for _, enum := range property.EnumMembers {
		if payloadEnum == enum.Name {
			return true
		}
	}

	// The enum was not found in the list of allowable values
	return false
}

// validateString will retrun true if the string meets the string requirements
// specified by the schema property
func checkString(payloadStringRaw interface{}, property *rfschema.Property) bool {
	payloadString, ok := payloadStringRaw.(string)
	// Check type correctness
	if !ok {
		return false
	}

	if property.StringFormat != nil && *property.StringFormat != rfschema.FormatUnknown {
		if !property.StringFormat.Validate(payloadString) {
			return false
		}
	}

	if property.StringMinLength != nil {
		if len(payloadString) < *property.StringMinLength {
			return false
		}
	}

	if property.StringMaxLength != nil {
		if *property.StringMaxLength < len(payloadString) {
			return false
		}
	}

	if property.StringPattern != nil {
		if !regexp.MustCompile(*property.StringPattern).MatchString(payloadString) {
			return false
		}
	}

	return true
}

// validateNumber will retrun true if the number meets the string requirements
// specified by the schema property
func checkNumber(payloadNumberRaw interface{}, property *rfschema.Property) bool {
	payloadNumber, ok := payloadNumberRaw.(float64)
	// Check type correctness
	if !ok {
		return false
	}
	if property.NumberMinimum != nil {
		if payloadNumber < *property.NumberMinimum {
			return false
		}
	}

	if property.NumberMaximum != nil {
		if *property.NumberMaximum < payloadNumber {
			return false
		}
	}

	return true
}

// validateBool will retrun true if the number meets the bool requirements
// specified by the schema property
func checkBool(payloadBoolRaw interface{}, property *rfschema.Property) bool {
	_, ok := payloadBoolRaw.(bool)
	return ok
}

// validateArray will check the array elements to see if they  meet the requirements
// specified by the element's schema property
func checkArray(payloadArray []interface{}, arrayProperty *rfschema.Property) ArrayReport {
	ar := NewArrayReport()
	elementProperty := arrayProperty.ArrayOf
	for i, payloadElementRaw := range payloadArray {
		switch elementProperty.Type {
		case rfschema.PropObject:
			payloadElement, ok := payloadElementRaw.(map[string]interface{})
			if ok {
				ar.Properties[i] = checkObject(payloadElement, elementProperty)
			} else {
				ar.InvalidElements[i] = true
			}
		case rfschema.PropAction:
			checkAction()
		case rfschema.PropEnum:
			ar.InvalidElements[i] = !checkEnum(payloadElementRaw, elementProperty)
		case rfschema.PropString:
			ar.InvalidElements[i] = !checkString(payloadElementRaw, elementProperty)
		case rfschema.PropNumber:
			ar.InvalidElements[i] = !checkNumber(payloadElementRaw, elementProperty)
		case rfschema.PropBool:
			ar.InvalidElements[i] = !checkBool(payloadElementRaw, elementProperty)
		case rfschema.PropArray:
			panic("Nested arrays are not supported")
		case rfschema.PropLink:
			if elementProperty.IsNavLink && elementProperty.LinkAutoExpand {
				// This collection is allowed to auto expanded
				// We are currently enumerating over the collection
				elementProperties := payloadElementRaw.(map[string]interface{})
				collection := elementProperty.Link
				ar.Properties[i] = checkProperties(elementProperties, collection.Properties, collection.AdditionalProperties, collection.PatternProperties)
			} else {
				ar.InvalidElements[i] = !checkLink(payloadElementRaw, elementProperty)
			}
		default:
			panic("Cannot handle type:" + elementProperty.Type.String())
		}
	}
	return ar
}

func checkLink(payloadLinkRaw interface{}, property *rfschema.Property) bool {
	payloadLink, ok := payloadLinkRaw.(map[string]interface{})
	if !ok {
		return false
	}

	// There should only be one element with the key of '@odata.id'
	if len(payloadLink) != 1 {
		return false
	}

	linkRaw, ok := payloadLink[`@odata.id`]
	if !ok {
		return false
	}

	link, ok := linkRaw.(string)
	if !ok {
		return false
	}

	return rfschema.FormatURI.Validate(link)
}
