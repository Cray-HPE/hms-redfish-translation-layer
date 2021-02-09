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

func copyResource(from *Resource) *Resource {
	if from == nil {
		panic("Nil argument")
	}

	to := &Resource{
		InstanceID: from.InstanceID,

		Name:                 from.Name,
		ODataType:            from.ODataType,
		ODataContext:         from.ODataContext,
		AdditionalProperties: from.AdditionalProperties,
		PatternProperties:    from.PatternProperties,
		IsCollection:         from.IsCollection,
		EmbeddedCollection:   from.EmbeddedCollection,
		RequiredProperties:   from.RequiredProperties,
		RequiredOnCreate:     from.RequiredOnCreate,
		VersionString:        from.VersionString,
		ReferenceCount:       from.ReferenceCount,
		Insertable:           from.Insertable,
		Updatable:            from.Updatable,
		Deletable:            from.Deletable,
	}

	if from.Description != nil {
		description := *from.Description
		to.Description = &description
	}

	if from.LongDescription != nil {
		longDescription := *from.LongDescription
		to.LongDescription = &longDescription
	}

	if from.CollectionOf != nil {
		panic("Collection can not link a resource yet")
	}

	to.Properties = map[string]*Property{}
	for name, fromProp := range from.Properties {
		to.Properties[name] = copyProperty(fromProp)
	}

	// Ignoring linkers & metadata

	return to
}

func copyProperty(from *Property) *Property {
	if from == nil {
		panic("Nil argument")
	}

	to := Property{
		Type:           from.Type,
		NullAllowed:    from.NullAllowed,
		ReadOnly:       from.ReadOnly,
		Deprecated:     from.Deprecated,
		linkRef:        from.linkRef,
		IsNavLink:      from.IsNavLink,
		IsNavArray:     from.IsNavArray,
		LinkAutoExpand: from.LinkAutoExpand,
	}

	if from.Properties != nil {
		to.Properties = map[string]*Property{}
		for name, fromProp := range from.Properties {
			to.Properties[name] = copyProperty(fromProp)
		}
	}

	// Types
	for _, typ := range from.Types {
		to.Types = append(to.Types, typ)
	}
	// PatternProperties
	for _, pattern := range from.PatternProperties {
		to.PatternProperties = append(to.PatternProperties, pattern)
	}

	// EnumMembers
	for _, enum := range from.EnumMembers {
		to.EnumMembers = append(to.EnumMembers, enum)
	}

	// DeprecatedEnums
	if from.DeprecatedEnums != nil {
		to.DeprecatedEnums = map[string]string{}
		for key, value := range from.DeprecatedEnums {
			to.DeprecatedEnums[key] = value
		}
	}

	if from.Link != nil {
		panic("Link has value")
	}

	// Ignoring metadata

	// TODO: This could probably be replaces with some refection thing
	// to check each "simple" field has a value and assign it
	if from.DeprecatedReason != nil {
		deprecatedReason := *from.DeprecatedReason
		to.DeprecatedReason = &deprecatedReason
	}

	if from.Description != nil {
		description := *from.Description
		to.Description = &description
	}

	if from.LongDescription != nil {
		longDescription := *from.LongDescription
		to.LongDescription = &longDescription
	}

	if from.AdditionalProperties != nil {
		additionalProperties := *from.AdditionalProperties
		to.AdditionalProperties = &additionalProperties
	}

	if from.arrayRefString != nil {
		arrayRefString := *from.arrayRefString
		to.arrayRefString = &arrayRefString
	}

	if from.StringFormat != nil {
		stringFormat := *from.StringFormat
		to.StringFormat = &stringFormat
	}

	if from.StringMaxLength != nil {
		stringMaxLength := *from.StringMaxLength
		to.StringMaxLength = &stringMaxLength
	}

	if from.StringMinLength != nil {
		stringMinLength := *from.StringMinLength
		to.StringMinLength = &stringMinLength
	}

	if from.StringPattern != nil {
		stringPattern := *from.StringPattern
		to.StringPattern = &stringPattern
	}

	if from.NumberMaximum != nil {
		numberMaximum := *from.NumberMaximum
		to.NumberMaximum = &numberMaximum
	}

	if from.NumberMinimum != nil {
		numberMinimum := *from.NumberMinimum
		to.NumberMinimum = &numberMinimum
	}

	if from.Units != nil {
		units := *from.Units
		to.Units = &units
	}

	if from.ArrayOf != nil {
		to.ArrayOf = copyProperty(from.ArrayOf)
	}

	to.Parameters = map[string]*Property{}
	for name, prop := range from.Parameters {
		to.Parameters[name] = copyProperty(prop)
	}

	for _, param := range from.RequiredParameters {
		to.RequiredParameters = append(to.RequiredParameters, param)
	}

	if from.Target != nil {
		to.Target = copyProperty(from.Target)
	}

	if from.Title != nil {
		to.Title = copyProperty(from.Title)
	}

	return &to
}
