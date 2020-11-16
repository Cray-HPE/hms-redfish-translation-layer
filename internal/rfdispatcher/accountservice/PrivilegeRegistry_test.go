/*
 * Privilege registry unit tests.
 *
 * Copyright 2018,2020 Hewlett Packard Enterprise Development LP
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
