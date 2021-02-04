// MIT License
//
// (C) Copyright [2019, 2021] Hewlett Packard Enterprise Development LP
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

package main

import (
	"fmt"
	"github.com/hashicorp/go-version"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ByVersion sorting functions for sorting an array of strings that are semantic versions.
type ByVersion []string

func (v ByVersion) Len() int {
	return len(v)
}
func (v ByVersion) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}
func (v ByVersion) Less(i, j int) bool {
	v1, err := version.NewVersion(strings.ReplaceAll(v[i], "_", "."))
	if err != nil {
		panic(err)
	}

	v2, err := version.NewVersion(strings.ReplaceAll(v[j], "_", "."))
	if err != nil {
		panic(err)
	}

	return v1.LessThan(v2)
}

func main() {
	// Just need a location to where the `contrib` folder is.
	contribDirPrefix, ok := os.LookupEnv("CONTRIB_DIR_PREFIX")
	if !ok {
		contribDirPrefix = "."
	}

	versionStr, ok := os.LookupEnv("SCHEMA_VERSION")
	if !ok {
		panic("SCHEMA_VERSION environment variable is not set")
	}

	// Compute the schema and mapping directories by pre-pending the contrib dir prefix.
	schemaPath := "contrib/DMTF/DSP8010-Redfish_Schema/DSP8010_" + versionStr + "/json-schema"
	schemaDir := filepath.Join(contribDirPrefix, schemaPath)

	fmt.Printf("Walking %s...\n", schemaDir)

	// Get a list of all the files in the schemaDir
	var jsonFiles []string

	_ = filepath.Walk(schemaDir, func(path string, info os.FileInfo, err error) error {
		jsonFiles = append(jsonFiles, path)
		return nil
	})

	// To make sure we don't run into the case where we encounter a file with a version before its root is found,
	// sort the array.
	sort.Strings(jsonFiles)

	// Build a list of just the root files.
	fileMap := make(map[string][]string)
	fileRegex := regexp.MustCompile(`(^\w+)\.(v.+)\.json`)

	for _, jsonFile := range jsonFiles {
		if jsonFile == schemaDir {
			continue
		}

		trimmedFile := strings.TrimPrefix(jsonFile, schemaPath+"/")
		// Add this to the right slice.
		fileSubmatches := fileRegex.FindStringSubmatch(trimmedFile)
		if len(fileSubmatches) < 3 {
			// If there are less than 3 parts to this slice then it's just the base file. Maps in Go are interesting,
			// they'll add keys as they need it which seems...dangerous, but cool.
			continue
		}

		fileMap[fileSubmatches[1]] = append(fileMap[fileSubmatches[1]], fileSubmatches[2])
	}

	fmt.Println("\nFiles and versions:")
	for schema, files := range fileMap {
		fmt.Printf("%s\n", schema)

		// Sort the files array using semantic version aware sorting to get newest at last index.
		sort.Sort(ByVersion(files))

		for _, file := range files {
			fmt.Printf("\t%s\n", file)
		}
	}

	// Print out JSONish representation.
	for schema, files := range fileMap {
		// Sort the files array using semantic version aware sorting to get newest at last index.
		sort.Sort(ByVersion(files))
		fmt.Printf("\"%s\": \"%s\",\n", schema + ".json", files[len(files) - 1])
	}
}
