/*
 * Account Service unit tests.
 *
 * Copyright 2018 Cray Inc.  All Rights Reserved
 *
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
