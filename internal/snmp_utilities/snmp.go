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
	"errors"
	"fmt"
	"strings"

	"github.com/k-sone/snmpgo"
)

const (
	OIDEntPhysicalDescr          string = "1.3.6.1.2.1.47.1.1.1.1.2"
	OIDEntPhysicalName           string = "1.3.6.1.2.1.47.1.1.1.1.7"
	OIDEntPhysicalHardwareRev    string = "1.3.6.1.2.1.47.1.1.1.1.8"
	OIDEntPhysicalFirmwareRev    string = "1.3.6.1.2.1.47.1.1.1.1.9"
	OIDEntPhysicalSoftwareRev    string = "1.3.6.1.2.1.47.1.1.1.1.10"
	OIDEntPhysicalSerialNum      string = "1.3.6.1.2.1.47.1.1.1.1.11"
	OIDEntPhysicalMfgName        string = "1.3.6.1.2.1.47.1.1.1.1.12"
	OIDEntPhysicalModelName      string = "1.3.6.1.2.1.47.1.1.1.1.13"
	OIDEntPhysicalAlias          string = "1.3.6.1.2.1.47.1.1.1.1.14"
	OIDEntPhysicalAssetID        string = "1.3.6.1.2.1.47.1.1.1.1.15"
	OIDEntPhysicalUUID           string = "1.3.6.1.2.1.47.1.1.1.1.19"
)

var OIDEntPhysicalEntryTable = []string{
	OIDEntPhysicalDescr,
	OIDEntPhysicalName,
	OIDEntPhysicalHardwareRev,
	OIDEntPhysicalFirmwareRev,
	OIDEntPhysicalSoftwareRev,
	OIDEntPhysicalSerialNum,
	OIDEntPhysicalMfgName,
	OIDEntPhysicalModelName,
	OIDEntPhysicalAlias,
	OIDEntPhysicalAssetID,
	OIDEntPhysicalUUID,
}

const (
	EntryTableIndexDescr       = 0
	EntryTableIndexName        = 1
	EntryTableIndexHardwareRev = 2
	EntryTableIndexFirmwareRev = 3
	EntryTableIndexSoftwareRev = 4
	EntryTableIndexSerialNum   = 5
	EntryTableIndexMfgName     = 6
	EntryTableIndexModelName   = 7
	EntryTableIndexAlias       = 8
	EntryTableIndexAssetID     = 9
	EntryTableIndexUUID        = 10
)

type EntityPhysicalTable struct {
	Index       string `json:"Index"`
	Descr       string `json:"Descr"`
	Name        string `json:"Name"`
	HardwareRev string `json:"HardwareRev"`
	FirmwareRev string `json:"FirmwareRev"`
	SoftwareRev string `json:"SoftwareRev"`
	SerialNum   string `json:"SerialNum"`
	MfgName     string `json:"MfgName"`
	ModelName   string `json:"ModelName"`
	Alias       string `json:"Alias"`
	AssetID     string `json:"AssetID"`
	UUID        string `json:"UUID"`
}

func NewSNMPInterface(address string, user string, authPassword string, authProtocol string, privPassword string, privProtocol string) (SNMPInterface, error) {
	var snmpInterface SNMPInterface

	// Check that the address ends in a port number (required by goSNMP).
	if !strings.Contains(address, ":") {
		address = fmt.Sprintf("%s:161", address)
	}

	var securityLevel snmpgo.SecurityLevel
	var authType snmpgo.AuthProtocol
	var privType snmpgo.PrivProtocol

	if strings.ToLower(authProtocol) == "none" {
		securityLevel = snmpgo.NoAuthNoPriv
	} else if strings.ToLower(privProtocol) == "none" {
		securityLevel = snmpgo.AuthNoPriv
	} else {
		securityLevel = snmpgo.AuthPriv
	}

	if securityLevel != snmpgo.NoAuthNoPriv {
		if strings.ToLower(authProtocol) == "md5" {
			authType = snmpgo.Md5
		} else if strings.ToLower(authProtocol) == "sha" {
			authType = snmpgo.Sha
		}
	}

	if securityLevel == snmpgo.AuthPriv {
		if strings.ToLower(privProtocol) == "aes" {
			privType = snmpgo.Aes
		} else if strings.ToLower(privProtocol) == "des" {
			privType = snmpgo.Des
		}
	}

	snmpObject, err := snmpgo.NewSNMP(snmpgo.SNMPArguments{
		Version:       snmpgo.V3,
		Address:       address,
		Retries:       5,
		UserName:      user,
		SecurityLevel: securityLevel,
		AuthPassword:  authPassword,
		AuthProtocol:  authType,
		PrivPassword:  privPassword,
		PrivProtocol:  privType,
	})

	snmpInterface = RealSNMP{
		SNMP: snmpObject,
	}

	return snmpInterface, err
}

func snmpGet(snmp *snmpgo.SNMP, oids []string, walk bool) (snmpgo.Pdu, error) {
	var result snmpgo.Pdu

	snmpOids, err := snmpgo.NewOids(oids)
	if err != nil {
		return nil, err
	}

	err = snmp.Open()
	if err != nil {
		return nil, err
	}
	defer snmp.Close()

	if walk {
		result, err = snmp.GetBulkWalk(snmpOids, 0, 10)
	} else {
		result, err = snmp.GetRequest(snmpOids)
	}
	if err != nil {
		return nil, err
	}

	if result.ErrorStatus() != snmpgo.NoError {
		return nil, errors.New(result.ErrorStatus().String())
	}

	return result, nil
}

// Get the SNMP index of the switch chassis component.
//
// As far as I can tell, the chassis is always the 2nd available component
// index (might not be contiguous). At least, that is what I observed for
// Aruba (index = 101001) and Dell/Mellenox (index = 2).
func snmpGetIndex(snmp *snmpgo.SNMP) (string, error) {
	// Use OIDEntPhysicalDescr because it is always there and easiest to
	// tell what component we're looking at.
	result, err := snmpGet(snmp, []string{OIDEntPhysicalDescr}, true)
	if err != nil {
		// Failed to request
		return "", err
	}

	oid, _ := snmpgo.NewOid(OIDEntPhysicalDescr)
	// Make sure we only have OIDs in the EntPhysicalDescr tree
	vbs := result.VarBinds().MatchBaseOids(oid)

	if len(vbs) < 2 {
		// There must be at least 2
		return "", errors.New("Unable to determine switch chassis index")
	}

	// As far as I can tell, the chassis is always the 2nd available component index (might not be contiguous).
	// At least that is what I observed for Aruba (index = 101001) and Dell/Mellenox (index = 2).
	// Get the index from the last part of the returned OIDs
	switchDescOID := vbs[1].Oid.String()
	parts := strings.Split(switchDescOID, ".")
	switchIndex := parts[len(parts)-1]

	return switchIndex, nil
}

func getPhysicalTableOIDsForIndex(index string) []string {
	tableOids := []string{}

	for _, oid := range OIDEntPhysicalEntryTable {
		tableOids = append(tableOids, oid + "." + index)
	}
	
	return tableOids
}

func (snmpInterface RealSNMP) GetPhysicalTable() (physTable EntityPhysicalTable, err error) {

	if snmpInterface.SnmpIndex == "" {
		snmpIndex, idxErr := snmpGetIndex(snmpInterface.SNMP)
		if idxErr != nil {
			err = fmt.Errorf("failed to get SNMP switch chassis index: %w", idxErr)
			return
		} else {
			snmpInterface.SnmpIndex = snmpIndex
		}
	}
	physicalTableOids := getPhysicalTableOIDsForIndex(snmpInterface.SnmpIndex)
	result, getErr := snmpGet(snmpInterface.SNMP, physicalTableOids, false)
	if getErr != nil {
		err = fmt.Errorf("failed to perform SNMP get: %w", getErr)
		return
	}

	vbs := result.VarBinds()

	physTable = EntityPhysicalTable{Index: snmpInterface.SnmpIndex}
	for i, oid := range physicalTableOids {
		snmpOid, _ := snmpgo.NewOid(oid)
		val := vbs.MatchOid(snmpOid).Variable.String()
		switch(i) {
		case EntryTableIndexDescr:
			physTable.Descr = val
		case EntryTableIndexName:
			physTable.Name = val
		case EntryTableIndexHardwareRev:
			physTable.HardwareRev = val
		case EntryTableIndexFirmwareRev:
			physTable.FirmwareRev = val
		case EntryTableIndexSoftwareRev:
			physTable.SoftwareRev = val
		case EntryTableIndexSerialNum:
			physTable.SerialNum = val
		case EntryTableIndexMfgName:
			physTable.MfgName = val
		case EntryTableIndexModelName:
			physTable.ModelName = val
		case EntryTableIndexAlias:
			physTable.Alias = val
		case EntryTableIndexAssetID:
			physTable.AssetID = val
		case EntryTableIndexUUID:
			physTable.UUID = val
		}
	}

	return
}
