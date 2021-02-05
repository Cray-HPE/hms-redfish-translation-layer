// MIT License
//
// (C) Copyright [2018,2020, 2021] Hewlett Packard Enterprise Development LP
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
 * Privilege registry unit tests.
 *
 */
package accountservice

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type PrivilegeRegistryTestSuite struct {
	suite.Suite
	pr *PrivilegeRegistry
}

func (suite *PrivilegeRegistryTestSuite) SetupTest() {
	contribDirPrefix, ok := os.LookupEnv("CONTRIB_DIR_PREFIX")
	if !ok {
		log.Panic("This requires the environment variable CONTRIB_DIR_PREFIX to be set")
	}

	privilegeRegistryVersion, ok := os.LookupEnv("PRIVILEGE_REGISTRY_VERSION")
	if !ok {
		log.Panic("This requires the environment variable PRIVILEGE_REGISTRY_VERSION to be set")
	}
	privilegeRegistryPath := filepath.Join(contribDirPrefix,
		"contrib/DMTF/DSP8011-Redfish_Standard_Registries/Redfish_"+privilegeRegistryVersion+"_PrivilegeRegistry.json")

	var err error
	suite.pr, err = NewPrivilegeRegistry(privilegeRegistryPath)
	if err != nil {
		suite.NoError(err)
	}
}

func (suite *PrivilegeRegistryTestSuite) TearDownTest() {

}

func TestPrivilegeRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(PrivilegeRegistryTestSuite))
}

func (suite *PrivilegeRegistryTestSuite) TestManagerAccountCollection() {
	// This test expects to be using version 1.0.4 of the Privilege Registry

	// Quick sanity test
	mapping, ok := suite.pr.GetMapping("ManagerAccountCollection")
	suite.True(ok)
	suite.NotNil(mapping)

	suite.Equal("ManagerAccountCollection", mapping.Entity)

	operationPrivileges := mapping.OperationMap[http.MethodGet]
	foundConfigureUsers := false
	foundConfigureManager := false
	foundLogin := false

	for _, op := range operationPrivileges {
		suite.Len(op.Privilege, 1)
		if op.Privilege[0] == "ConfigureUsers" {
			foundConfigureUsers = true
		}
		if op.Privilege[0] == "ConfigureManager" {
			foundConfigureManager = true
		}
		if op.Privilege[0] == "Login" {
			foundLogin = true
		}
	}

	suite.True(foundConfigureUsers)
	suite.True(foundConfigureManager)
	suite.False(foundLogin)
}
