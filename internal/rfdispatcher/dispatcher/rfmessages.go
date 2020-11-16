/*
 * Redfish Message Registries.
 * This file parses message registries and allows the generation of redfish messages.
 *
 * Copyright 2018 Cray Inc.  All Rights Reserved
 *
 */
package dispatcher

import (
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"path"
	"path/filepath"
	"strings"
)

const (
	RegCopyrightKey   = "@Redfish.Copyright"
	RegTypeKey        = "@odata.type"
	RegIDKey          = "Id"
	RegNameKey        = "Name"
	RegLanguageKey    = "Language"
	RegDescriptionKey = "Description"
	RegPrefixKey      = "RegistryPrefix"
	RegVersionKey     = "RegistryVersion"
	RegEntityKey      = "OwningEntity"
	RegMessagesKey    = "Messages"
)

const (
	MsgRegIDKey            = "Id"
	MsgRegDescriptionKey   = "Description"
	MsgRegMessageKey       = "Message"
	MsgRegNumArgsKey       = "NumberOfArgs"
	MsgRegArgTypeKey       = "ParamTypes"
	MsgRegResolutionKey    = "Resolution"
	MsgRegClearingLogicKey = "ClearingLogic"
	MsgRegSeverityKey      = "Severity"
)

/* String constants from Message.v1_0_6.json */
const (
	MsgMessageKey     = "Message"
	MsgMessageArgsKey = "MessageArgs"
	MsgMessageIdKey   = "MessageId"
	MsgResolutionKey  = "Resolution"
	MsgSeverityKey    = "Severity"
)

var RegistryDirs = []string{
	"contrib/Cray/hms-controllers/etc/message_registries",
	"contrib/DMTF/DSP8011-Redfish_Standard_Registries",
}

var SupportedRegistryIDs = []string{
	"Base.1.4.0",
	"CrayTelemetry.1.0.0",
}

// TODO: The checks on these keys could just come from the schema
var KnownRegistryKeys = []string{
	RegCopyrightKey,
	RegTypeKey,
	RegIDKey,
	RegNameKey,
	RegLanguageKey,
	RegDescriptionKey,
	RegPrefixKey,
	RegVersionKey,
	RegEntityKey,
	RegMessagesKey,
}

var KnownMessageKeys = []string{
	MsgRegIDKey,
	MsgRegDescriptionKey,
	MsgRegMessageKey,
	MsgRegNumArgsKey,
	MsgRegArgTypeKey,
	MsgRegResolutionKey,
	MsgRegClearingLogicKey,
	MsgRegSeverityKey,
}

/* This is based off of Message.v1_0_6.json, we may want to send "leaner"
 * messages */
type Message struct {
	Message     string
	MessageArgs []string
	MessageId   string
	Resolution  string
	Severity    string
}

func (rfd *RedfishDispatcher) InitMessageRegistries() {
	rfd.messageRegistries = map[string]interface{}{}

	registryFiles := []string{}

	for _, regDir := range RegistryDirs {
		regPath := path.Join(rfd.SchemaConfig.ContribDirPrefix, regDir)
		matches, err := filepath.Glob(regPath + "/*.json")
		if err != nil {
			panic(err)
		}
		registryFiles = append(registryFiles, matches...)
	}

	if len(registryFiles) == 0 {
		log.WithField("RegistryDirs", RegistryDirs).Fatal(
			"Failed to find any JSON files in supplied directories")
	}

	for _, registryFile := range registryFiles {
		messageRegistryRaw, err := ioutil.ReadFile(registryFile)
		if err != nil {
			panic(err)
		}
		unNamedRegistry := map[string]interface{}{}
		err = json.Unmarshal(messageRegistryRaw, &(unNamedRegistry))
		if err != nil {
			panic(err)
		}

		// Validate the message registry against the schema
		for _, knownKey := range KnownRegistryKeys {
			if _, ok := unNamedRegistry[knownKey]; !ok {
				log.WithFields(log.Fields{
					"knownKey":     knownKey,
					"registryFile": registryFile,
				}).Trace("JSON file missing key")
				continue
			}
		}

		for regKey := range unNamedRegistry {
			present := false
			for _, knownKey := range KnownRegistryKeys {
				if regKey != knownKey {
					present = true
				}
			}
			if !present {
				log.WithFields(log.Fields{
					"regKey":       regKey,
					"registryFile": registryFile,
				}).Trace("JSON file missing key")
				continue
			}
		}

		RegistryIDValid := false
		regID := unNamedRegistry[RegIDKey]
		for _, supportedID := range SupportedRegistryIDs {
			if regID == supportedID {
				RegistryIDValid = true
				break
			}
		}

		// For the initial version these are the only key's we'll check/store
		logFields := log.Fields{
			"regID":        regID,
			"registryFile": registryFile,
		}
		if !RegistryIDValid {
			log.WithFields(logFields).Trace("Unsupported registry ID")
		} else {
			log.WithFields(logFields).Info("Found supported message registry")
			// Now that we've checked the registry and gotten the name, add to map
			key := unNamedRegistry[RegPrefixKey].(string)
			rfd.messageRegistries[key] = unNamedRegistry
		}
	}
}

func (rfd *RedfishDispatcher) GetRedfishMessage(registryPrefix string, messageID string, args ...interface{}) (map[string]interface{}, error) {
	var ok bool
	var messageRegistry map[string]interface{}
	var regMsg map[string]interface{}
	msg := map[string]interface{}{}

	if messageRegistry, ok = rfd.messageRegistries[registryPrefix].(map[string]interface{}); !ok {
		return nil, errors.New("Registry not found:" + registryPrefix)
	}

	msgsRaw := messageRegistry[RegMessagesKey]
	msgs := msgsRaw.(map[string]interface{})

	if regMsg, ok = msgs[messageID].(map[string]interface{}); !ok {
		return nil, errors.New("MessageID not found")
	}

	msg[MsgMessageKey] = regMsg[MsgRegMessageKey]
	verDigits := strings.Split(messageRegistry[RegVersionKey].(string), ".")
	s := []string{registryPrefix, verDigits[0], verDigits[1], messageID}
	msg[MsgMessageIdKey] = strings.Join(s, ".")
	msg[MsgResolutionKey] = regMsg[MsgRegResolutionKey]
	msg[MsgSeverityKey] = regMsg[MsgRegSeverityKey]

	if regMsg[MsgRegNumArgsKey].(float64) > float64(len(args)) {
		return nil, errors.New("Not enough arguments provided")
	}

	if regMsg[MsgRegNumArgsKey].(float64) < float64(len(args)) {
		return nil, errors.New("Too many arguments arguments provided")
	}

	if regMsg[MsgRegNumArgsKey].(float64) > 0 {
		msg[MsgMessageArgsKey] = []interface{}{}
	}
	for i, arg := range args {
		var s_val string
		argPattern := fmt.Sprintf("%%%v", int(i+1))
		switch val := arg.(type) {
		case int:
			if (regMsg[MsgRegArgTypeKey].([]interface{}))[i] != "number" {
				return nil, errors.New("Int provided for non-number ParamType")
			}
			s_val = fmt.Sprintf("%v", int(val))
		case string:
			if (regMsg[MsgRegArgTypeKey].([]interface{}))[i] != "string" {
				return nil, errors.New("String provided for non-string ParamType")
			}
			s_val = val
		default:
			return nil, errors.New("Unsupported data type")
		}
		msg[MsgMessageKey] = strings.Replace(msg[MsgMessageKey].(string), argPattern, s_val, -1)
		msg[MsgMessageArgsKey] = append(msg[MsgMessageArgsKey].([]interface{}), s_val)
	}

	return msg, nil

}

func (rfd *RedfishDispatcher) GetRedfishMessageString(registryPrefix string, messageID string, args ...interface{}) (string, error) {
	msg, err := rfd.GetRedfishMessage(registryPrefix, messageID, args...)
	if err != nil {
		return "", err
	}

	jsonMsgBytes, err := json.Marshal(msg)
	if err == nil {
		return string(jsonMsgBytes), nil
	}
	return "", errors.New("Error: Failed to convert Message to JSON")
}
