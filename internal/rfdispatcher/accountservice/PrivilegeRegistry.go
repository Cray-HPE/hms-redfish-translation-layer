// MIT License
//
// (C) Copyright [2018, 2020-2021] Hewlett Packard Enterprise Development LP
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
 * Privilege registry.
 */
package accountservice

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	log "github.com/sirupsen/logrus"
)

type PrivilegeRegistry struct {
	RedfishCopyright string `json:"@Redfish.Copyright"`
	ODataType        string `json:"@odata.type"`

	Id   string
	Name string

	PrivilegesUsed    []string
	OEMPrivilegesUsed []string

	Mappings []Mapping

	// Built by the NewPrivilegeRegistry
	entityMappings map[string]Mapping
}

// NewPrivilegeRegistry creates a new Privilege Registry from the specified file
func NewPrivilegeRegistry(filePath string) (*PrivilegeRegistry, error) {
	contents, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var pr PrivilegeRegistry
	err = json.Unmarshal(contents, &pr)
	if err != nil {
		return nil, err
	}

	pr.entityMappings = map[string]Mapping{}
	for _, mapping := range pr.Mappings {
		pr.entityMappings[mapping.Entity] = mapping
	}

	return &pr, nil
}

type Mapping struct {
	Entity string

	OperationMap         map[string][]OperationPrivilege
	PropertyOverrides    []TargetPrivilegeMap
	ResourceURIOverrides []TargetPrivilegeMap // TOOD not in default Privilege Registry
	SubordinateOverrides []TargetPrivilegeMap
}

type OperationPrivilege struct {
	Privilege []string
}

type TargetPrivilegeMap struct {
	Targets      []string
	OperationMap map[string][]OperationPrivilege
}

func (pr *PrivilegeRegistry) GetMapping(name string) (Mapping, bool) {
	mapping, ok := pr.entityMappings[name]
	return mapping, ok
}

// RequiredPrivileges will determine what privileges are required for an operation
// One of the OperationPrivileges must be satisfied for an account in the returned array.
func (pr *PrivilegeRegistry) RequiredPrivileges(httpOp, owningEntity, entity, property string) []OperationPrivilege {
	mapping, ok := pr.entityMappings[entity]
	if !ok {
		log.WithField("entity", entity).Error("Entity does not exist in Privilege Registry")
		panic(fmt.Errorf("Entity does not exist in Privilege Registry: %s", entity))
	}

	// Look For overrides
	// - Property Overide
	// - Subordinate override
	for _, propertyOverride := range mapping.PropertyOverrides {
		for _, target := range propertyOverride.Targets {
			if target == property {
				// Found overridden property
				result, ok := propertyOverride.OperationMap[httpOp]
				if !ok {
					log.Debug("Property override does not contain HTTP operation")
					break
				}
				log.WithField("result", result).Trace("Found property override")
				return result
			}
		}
	}

	for _, subordinateOverride := range mapping.SubordinateOverrides {
		for _, target := range subordinateOverride.Targets {
			if target == owningEntity {
				// Found owning entity override
				result, ok := subordinateOverride.OperationMap[httpOp]
				if !ok {
					log.Debug("Property override does not contain HTTP operation")
				}
				log.WithField("result", result).Trace("Found Subordinate override")
				return result
			}
		}
	}

	// TODO Resource URI Overrides

	privileges, ok := mapping.OperationMap[httpOp]
	if !ok {
		log.WithField("httpOp", httpOp).Fatal("Operation map does contain http operation")
	}
	log.WithField("privileges", privileges).Trace("No overrides found")

	log.WithFields(log.Fields{
		"httpOp":       httpOp,
		"owningEntity": owningEntity,
		"entity":       entity,
		"property":     property,
	}).Debug("Required privileges found")

	return privileges
}
