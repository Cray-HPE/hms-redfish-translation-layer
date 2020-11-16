package rfschema

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Version is a enumeration of the released DMTF Redfish Schema Bundles
type Version int

// Enumeration of the released DMTF Schema bundles
const (
	VersionUnknown Version = iota
	Version1_0
	Version1_0_1
	Version1_1
	Version2016_1
	Version2016_2
	Version2016_3
	Version2017_1
	Version2017_2
	Version2017_3
	Version2018_1
	Version2018_2
	Version2018_3
	Version2019_1
	VersionLatest // No mapping is required for latest, as the highest version is always used
)

func (v Version) String() string {
	switch v {
	case Version1_0:
		return "Version 1.0"
	case Version1_0_1:
		return "Version 1.0.1"
	case Version1_1:
		return "Version 1.1"
	case Version2016_1:
		return "Version 2016.1"
	case Version2016_2:
		return "Version 2016.2"
	case Version2016_3:
		return "Version 2016.3"
	case Version2017_1:
		return "Version 2017.1"
	case Version2017_2:
		return "Version 2017.2"
	case Version2017_3:
		return "Version 2017.3"
	case Version2018_1:
		return "Version 2018.1"
	case Version2018_2:
		return "Version 2018.2"
	case Version2018_3:
		return "Version 2018.3"
	case Version2019_1:
		return "Version 2019.1"
	case VersionLatest:
		return "Version Latest"
	default:
		return "Unknown"
	}
}

func ParseVersion(name string) (Version, error) {
	mapping := map[string]Version{
		"1.0":    Version1_0,
		"1.0.1":  Version1_0_1,
		"1.1":    Version1_1,
		"2016.1": Version2016_1,
		"2016.2": Version2016_2,
		"2016.3": Version2016_3,
		"2017.1": Version2017_1,
		"2017.2": Version2017_2,
		"2017.3": Version2017_3,
		"2018.1": Version2018_1,
		"2018.2": Version2018_2,
		"2018.3": Version2018_3,
		"2019.1": Version2019_1,
		"Latest": VersionLatest,
	}

	if version, ok := mapping[name]; ok {
		return version, nil
	}
	return VersionUnknown, errors.New(name + " is not a valid version")
}

type VersionMapping struct {
	Version            Version           `json:"Version"`
	Resources          map[string]string `json:"Resources"`
	Objects            map[string]string `json:"Objects"`
	AllowedCollections map[string]bool   `json:"AllowedCollections"`
	KnownCycles        map[string]Cycle  `json:"KnownCycles"` // map[SourceResource]Cycle
}

type Cycle struct {
	SourceField         string `json:"SourceField"`
	DestinationResource string `json:"DestinationResource"`
}

// func (vm *VersionMapping) Get(name string) (string, bool) {
// 	version, ok := vm.Resources[name]
// 	return version, ok
// }

type VersionMapper struct {
	mappings map[Version]VersionMapping
}

var mapperSingleton *VersionMapper
var mapperOnce sync.Once

func GetVersionMapper() *VersionMapper {
	mapperOnce.Do(func() {
		mapperSingleton = &VersionMapper{
			mappings: make(map[Version]VersionMapping),
		}
	})
	return mapperSingleton
}

func (vm *VersionMapper) AddMapping(mapping VersionMapping) error {
	if _, present := vm.mappings[mapping.Version]; present {
		return errors.New("Version mapping already exists")
	}

	// Add it to known mappings
	vm.mappings[mapping.Version] = mapping

	return nil
}

func (vm *VersionMapper) AddMappingFromFile(filePath string) error {
	mappingRaw, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	mapping := VersionMapping{}
	err = json.Unmarshal(mappingRaw, &mapping)
	if err != nil {
		return err
	}

	return vm.AddMapping(mapping)
}

func (vm *VersionMapper) ReadDir(mappingDir string) error {
	debugPrintln(0, "Reading mapping dir:", mappingDir)
	files, err := ioutil.ReadDir(mappingDir)
	if err != nil {
		return errors.New("Unable to read mapping directory. Reason: " + err.Error())
	}

	for _, file := range files {
		debugPrintln(0, "Adding map:", file.Name())
		filePath := path.Join(mappingDir, file.Name())

		err = vm.AddMappingFromFile(filePath)
		if err != nil {
			return errors.New("Unable to add mapping file: " + filePath + " Reason: " + err.Error())
		}
	}

	return nil
}

func (vm *VersionMapper) GetMapping(version Version) (VersionMapping, bool) {
	mapping, ok := vm.mappings[version]
	return mapping, ok
}

// selectResourceVersion will attempt to choose the version of the resource schema file that corresponds to the
// selected DMTF Schema Version
func selectResourceVersion(schemaName string, version Version, versions []string) string {
	if len(versions) == 0 {
		panic("No versions present")
	}

	if version == VersionLatest {
		// Just the highest version
		sort.Strings(versions)
		return versions[len(versions)-1]
	} else if mapping, ok := GetVersionMapper().GetMapping(version); ok {
		if version, ok := mapping.Resources[schemaName]; ok {
			debugPrintln(0, schemaName, "->", version)
			return version
		}
		panic("Resource not found: " + schemaName)
	} else {
		panic("Unsupported version: " + version.String() + " for: " + schemaName)
	}
}

func selectObjectVersion(schemaName string, version Version, versions []string) string {
	if len(versions) == 0 {
		panic("No versions present")
	}

	if version == VersionLatest {
		// Just the highest version
		sort.Strings(versions)
		return versions[len(versions)-1]
	} else if mapping, ok := GetVersionMapper().GetMapping(version); ok {
		if version, ok := mapping.Objects[schemaName]; ok {
			debugPrintln(0, schemaName, "->", version)
			return version
		}

		debugPrintln(0, "Checking resource version mappings")

		// Is the versioned object in a resource schema?
		// Such as StorageController in Storage schemas
		if version, ok := mapping.Resources[schemaName]; ok {
			debugPrintln(0, schemaName, "->", version)
			return version
		}

		panic("Object not found: " + schemaName)
	} else {
		panic("Unsupported version: " + version.String() + " for: " + schemaName)
	}
}

// ExtractResourceVersions will extract all of the allowable resource version from the top level resource.
// Ex: To get the allowed versions of chassis look into Chassis.json (the top level resource)
func ExtractResourceVersions(object map[string]interface{}) map[string]string {
	version2Ref := make(map[string]string) // Version -> $ref
	if rawAnyOf, ok := object[KeyAnyOf]; ok {
		anyOf := rawAnyOf.([]interface{})
		for _, rawElement := range anyOf {
			element := rawElement.(map[string]interface{})
			if rawRef, ok := element[KeyRef]; ok {
				refString := rawRef.(string)

				if strings.Contains(refString, "idRef") {
					// Skip http://redfish.dmtf.org/schemas/v1/odata.v4_0_2.json#/definitions/idRef
					debugPrintln(0, "Ignoring idref")
					continue
				}

				version := regexp.MustCompile("v[0-9]_[0-9]_[0-9]").FindString(refString)
				if version == "" {
					// The referenced schema should contain its version
					panic("Could not find version from reference")
				}

				version2Ref[version] = refString
			} else {
				panic("Expected reference")
			}
		}
	}

	// spew.Dump(version2Ref)
	return version2Ref
}
