// MIT License
//
// (C) Copyright [2020-2021] Hewlett Packard Enterprise Development LP
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
	"testing"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

type Walk_TS struct {
	suite.Suite
	tree         *Tree
	collectionCB CollectionCB
}

func (suite *Walk_TS) SetupSuite() {
	// Configuration from environment variables
	schemaConfig, err := SetupEnvConfig()
	if err != nil {
		panic(err)
	}

	// Next load in the Redfish Schema Tree
	suite.tree, err = LoadTreeFromConfig(schemaConfig)
	if err != nil {
		log.WithFields(log.Fields{
			"err":          err,
			"schemaConfig": schemaConfig,
		}).Error("Failed to load schema tree")
		panic(err)
	}

	// For simplicity all collection members will have the name *
	suite.collectionCB = func(collection *Resource, collectionPath string, instance string) bool {
		return instance == "*"
	}
}

func (suite *Walk_TS) Test_WalkManagerCollection() {
	path := "/redfish/v1/Managers"
	resource, property := WalkTree(suite.tree, suite.collectionCB, path)
	suite.NotNil(resource)
	suite.Nil(property)

	suite.Equal(resource.Name, "ManagerCollection")
	suite.True(resource.IsCollection)

}

func (suite *Walk_TS) Test_WalkManagerBMC() {
	path := "/redfish/v1/Managers/*"
	resource, property := WalkTree(suite.tree, suite.collectionCB, path)
	suite.NotNil(resource)
	suite.Nil(property)

	suite.Equal(resource.Name, "Manager")
	suite.False(resource.IsCollection)

}

func (suite *Walk_TS) Test_WalkManagerBMC_NetworkProtocol() {
	path := "/redfish/v1/Managers/*/NetworkProtocol"
	resource, property := WalkTree(suite.tree, suite.collectionCB, path)
	suite.NotNil(resource)
	suite.Nil(property)

	suite.Equal(resource.Name, "ManagerNetworkProtocol")
	suite.False(resource.IsCollection)

	// Now Lets check that the navigation
	httpsProperty, ok := resource.Properties["HTTPS"]
	suite.True(ok)

	certificateProperty, ok := httpsProperty.Properties["Certificates"]
	suite.True(ok)
	suite.NotNil(certificateProperty)

	suite.Equal(PropLink, certificateProperty.Type)
	suite.True(certificateProperty.IsNavLink, "Certificate Link is not a nav link")
	suite.NotEmpty(certificateProperty.linkRef, "linkRef is the empty string")
	suite.NotNil(certificateProperty.Link, "Link is nil")
}

func (suite *Walk_TS) Test_WalkManagerHTTPSCertificateCollection() {
	path := "/redfish/v1/Managers/*/NetworkProtocol/HTTPS/Certificates"
	resource, property := WalkTree(suite.tree, suite.collectionCB, path)
	suite.NotNil(resource)
	suite.Nil(property)

	suite.Equal(resource.Name, "CertificateCollection")
	suite.True(resource.IsCollection)
}

func Test_Walk(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)
	suite.Run(t, new(Walk_TS))
}
