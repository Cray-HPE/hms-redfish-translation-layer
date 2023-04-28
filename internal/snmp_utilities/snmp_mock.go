// MIT License
//
// (C) Copyright [2023] Hewlett Packard Enterprise Development LP
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

package snmp_utilities

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

func GetSNMPMockInterface(name string) (SNMPInterface, error) {
	var snmpInterface SNMPInterface
	snmpInterface = MockSNMP{
		SwitchXname: name,
	}
	return snmpInterface, nil
}

// This just exists to make development easier. Testing with a real switch is a pain, so it's nice to just capture
// the output from a switch one, dump it into some files, and then just pass those files right back when testing.
func (snmpInterface MockSNMP) GetPhysicalTable() (physTable EntityPhysicalTable, err error) {
	jsonFile, err := os.Open("configs/snmpSwitchPhysicalTable.json")
	if err != nil {
		return
	}

	jsonBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return
	}

	var mockPhysTable map[string]EntityPhysicalTable
	err = json.Unmarshal(jsonBytes, &mockPhysTable)

	if err != nil {
		return
	}

	var found bool
	physTable, found = mockPhysTable[snmpInterface.SwitchXname]

	if !found {
		err = fmt.Errorf("switch xname not found")
	}

	return
}
