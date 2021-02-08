// MIT License
//
// (C) Copyright [2018-2021] Hewlett Packard Enterprise Development LP
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
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Resource contains the schema for a Redfish Resource.
// It can be either an collection or a resource accessible schema.
//
// IMPORTANT: If any change is made made this structure make sure to modify `copy.go`!
type Resource struct {
	// Common
	Name                 string
	ODataType            string // Format: #Namespace.Type
	ODataContext         string // Format: /redfish/v1/$metadata#Namespace.Entity This can be derived from the unversioned schema
	Description          *string
	LongDescription      *string
	AdditionalProperties bool
	// Specify valid names for properties when AdditionalProperties is true. See PatternProperties for more infomation
	PatternProperties []PatternProperties

	Deletable  bool // Is the DELETE method allowed
	Insertable bool // Is the POST method allowed
	Updatable  bool // Are the PATCH or PUT methods allowed

	// Collection
	IsCollection       bool
	CollectionOf       *Resource // Not populated until linking stage
	EmbeddedCollection bool      // This collection is embedded.

	// Resource
	Properties         map[string]*Property
	RequiredProperties []string // Required properties for resource, such as: ["ChassisType", "Id", "Name"]
	RequiredOnCreate   []string // Required properties on resource creation
	VersionString      string   // Derived from the schema file name. Such as: v1_0_0

	// Application Specific data
	// The following fields are not derived from the schema
	ApplicationData map[string]interface{}

	// Internal data
	// The following fields are not derived from the schema
	ReferenceCount int
	Linkers        []*Resource
	InstanceID     int
}

// PropertyType is a enumeration for valid property types
type PropertyType int

func (t PropertyType) String() string {
	switch t {
	case PropUnknown:
		return "Unknown"
	case PropObject:
		return "Object"
	case PropAction:
		return "Action"
	case PropEnum:
		return "Enum"
	case PropString:
		return "String"
	case PropNumber:
		return "Number"
	case PropInteger:
		return "Integer"
	case PropBool:
		return "Bool"
	case PropArray:
		return "Array"
	case PropLink:
		return "Link"
	case PropMultiple:
		return "Multiple"
	default:
		return "Unrecognized enum"
	}
}

// Enumeration of valid property types
const (
	PropUnknown PropertyType = iota
	PropMultiple
	PropObject
	PropAction
	PropEnum
	PropString
	PropNumber
	PropInteger
	PropBool
	PropArray
	PropLink // Link to another Resource
)

// StringFormat is a enumeration of valid string formats
type StringFormat int

// Validate will check to see if the given string is valid
// Will always return false if the format is Unknown
func (sf StringFormat) Validate(str string) bool {
	switch sf {
	case FormatDate:
		// If this does not error out, then it is a valid date-time string
		_, err := time.Parse(time.RFC3339, str)
		return err == nil
	case FormatURI:
		// If this does not error out, then it is a valid uri string
		_, err := url.ParseRequestURI(str)
		return err == nil
	}
	return false
}

// Enumeration of valid string formats
const (
	FormatUnknown StringFormat = iota
	FormatDate                 // Conforms to RFC 3339
	FormatURI
)

// Property represents some property of a resource or object. See PropertyType for valid types.
// Only fields that are appropriate for a type of property should be set.
// There is no that all of the fields for a specific property type will be non nil.
// NOTE: If any changed is made made this structure make sure to modify copy.go
type Property struct {
	// Common
	Type             PropertyType
	NullAllowed      bool
	ReadOnly         bool
	Deprecated       bool
	DeprecatedReason *string

	Description     *string
	LongDescription *string

	// Multiple
	Types []PropertyType

	// Action
	Parameters         map[string]*Property // map[Parameter Name]*Property
	RequiredParameters []string
	Target             *Property
	Title              *Property

	// Link
	Link           *Resource // If nil then this is a generic IDref
	linkRef        string    // JSON Reference to the type linked
	IsNavLink      bool      // Is this a link that is used to navigate the tree. If false, just a link to the resource type.
	LinkAutoExpand bool      // Is this array a collection? Examples include: Thermal->Fans

	// Object
	Properties           map[string]*Property
	AdditionalProperties *bool
	PatternProperties    []PatternProperties // See PatternProperties for more details.

	// Array
	ArrayOf        *Property // Describes the properties of the elements
	IsNavArray     bool      // This array contains an array of navigation links
	arrayRefString *string   // Only set if this array points to an reference. Used to help determine the OData type for a embedded collection

	// String
	StringFormat    *StringFormat
	StringMaxLength *int
	StringMinLength *int
	StringPattern   *string // regex

	// Enum
	EnumMembers     []EnumMember
	DeprecatedEnums map[string]string // Enum name -> Reason

	// Numeric types, shared between Number and Integer
	Units *string

	// Number
	NumberMaximum *float64
	NumberMinimum *float64

	// Integer
	IntegerMaximun *int
	IntegerMinimun *int

	// Bool

	// Application Specific data
	// The following fields are not derived from the schema
	ApplicationData map[string]interface{}
}

// EnumMember is a member of some enumeration
// Example: The following  is a enum from the schema
// "IndicatorLED": {
// 	"enum": ["Unknown", "Lit", "Blinking", "Off"]
// 	],
// 	"enumDeprecated": {
// 		"Unknown": "This value has been Deprecated in favor of returning null if the state is unknown."
// 	},
// 	"enumDescriptions": {
// 		"Blinking": "The Indicator LED is blinking.",
// 		"Lit": "The Indicator LED is lit.",
// 		"Off": "The Indicator LED is off.",
// 		"Unknown": "The state of the Indicator LED cannot be determined."
// 	},
//  .....
// },
type EnumMember struct {
	Name            string // Enumeration name such as Unknown, Lit, Blinking, or Off in IndicatorLED
	Description     string
	LongDescription string
}

// PatternProperties is used by objects and resources to specify valid names for
//  properties that are not defined in the schema. If the `AdditionalProperties`
// is set to false in an object or resource and the name of property matches the
// regular expression in the field `Pattern`. Then the property
//
// Example: The following is a pattern properties definition used by the Chassis schema
//  "patternProperties": {
// 	"^([a-zA-Z_][a-zA-Z0-9_]*)?@(odata|Redfish|Message)\\.[a-zA-Z_][a-zA-Z0-9_.]+$": {
// 		"description": "This property shall specify a valid odata or Redfish property.",
// 		"type": [
// 			"array",
// 			"boolean",
// 			"number",
// 			"null",
// 			"object",
// 			"string"
// 		]
// 	}
// },
type PatternProperties struct {
	Pattern     string // Regex
	Description string
	Types       []string
}

// Regex will compile the Pattern regular expression
func (pp *PatternProperties) Regex() *regexp.Regexp {
	return regexp.MustCompile(pp.Pattern)
}

func debugPrintln(level int, args ...interface{}) {
	newArgs := []interface{}{strings.Repeat("  ", level)}
	newArgs = append(newArgs, args...)
	logrus.Trace(newArgs)
}
