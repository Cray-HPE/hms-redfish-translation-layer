// MIT License
//
// (C) Copyright [2018, 2021] Hewlett Packard Enterprise Development LP
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
 * Account Service unit tests.
 */

package accountservice

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type AccountServiceTestSuite struct {
	suite.Suite
}

func (suite *AccountServiceTestSuite) SetupTest() {

}

func (suite *AccountServiceTestSuite) TearDownTest() {

}

func TestAccountServiceTestSuite(t *testing.T) {
	suite.Run(t, new(AccountServiceTestSuite))
}

func (suite *AccountServiceTestSuite) TestRole() {
	role := Role{
		RoleId:             "ReadOnly",
		AssignedPrivileges: []string{"Login", "ConfigureSelf"},
	}

	// Test: User has privilege (single)
	required := []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"ConfigureSelf"},
		},
	}
	suite.True(role.HasPrivilege(required))

	// Test: User does not have privilege (single)
	required = []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"ConfigureManager"},
		},
	}
	suite.False(role.HasPrivilege(required))

	// Test: User has both privileges
	required = []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"Login", "ConfigureSelf"},
		},
	}
	suite.True(role.HasPrivilege(required))

	// Test: User is missing a privilege
	required = []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"Login", "ConfigureManager"},
		},
	}
	suite.False(role.HasPrivilege(required))

	// Test: User meets one of the Operation Privileges
	required = []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"Login"},
		},
		OperationPrivilege{
			Privilege: []string{"ConfigureManager"},
		},
	}
	suite.True(role.HasPrivilege(required))

	// Test: User meets neither of the Operation Privileges
	required = []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"ConfigureUsers"},
		},
		OperationPrivilege{
			Privilege: []string{"ConfigureManager"},
		},
	}
	suite.False(role.HasPrivilege(required))

	// Test: User meets both privileges of one of the Operation Privileges
	required = []OperationPrivilege{
		OperationPrivilege{
			Privilege: []string{"Login", "ConfigureSelf"},
		},
		OperationPrivilege{
			Privilege: []string{"ConfigureManager"},
		},
	}
	suite.True(role.HasPrivilege(required))

}
