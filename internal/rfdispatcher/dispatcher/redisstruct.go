// MIT License
//
// (C) Copyright [2019-2021] Hewlett Packard Enterprise Development LP
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
 * Helper functions to convert Resources and objects stored in Redis into  Golang map[string]interfaces{}, and vice
 * versa.
 * If a field's value requires a dispatch these functions will execute it.
 *
 */
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cray-HPE/hms-redfish-translation-service/internal/backend_helpers"
	_ "github.com/Cray-HPE/hms-redfish-translation-service/internal/logger"
	"github.com/Cray-HPE/hms-redfish-translation-service/internal/rfschema"

	"github.com/go-redis/redis"

	log "github.com/sirupsen/logrus"
)

// All simple types are stored in redis as strings and can be acquired in the same method, with the Type only used
// for JSON encoding.
func isRedfishValueString(pType rfschema.PropertyType) bool {
	if pType == rfschema.PropEnum || pType == rfschema.PropString ||
		pType == rfschema.PropNumber || pType == rfschema.PropInteger ||
		pType == rfschema.PropBool {
		return true
	}
	return false
}

/* Redis2Interface is used to build structures from redis.
 * Use the function NewRedis2Interface to initialize a instance of this structure. Each time a different resource is
 * being built a new instance should be used.*/
type Redis2Interface struct {
	RFD      *RedfishDispatcher
	Resource *rfschema.Resource
	URI      string
	XName    string
}

func (r2s *Redis2Interface) redis() *redis.Client {
	return r2s.RFD.redis
}

// Generates OData annotations for a given key if applicable for that key.
// If the key is unknown or not applicable then an error is returned.
func (r2s *Redis2Interface) generateOData(key string) (string, error) {
	trimmedURI := strings.TrimRight(r2s.URI, "/")
	memberName := filepath.Base(key)

	if memberName == "@odata.id" {
		// We don't want any URIs coming back with xname information prefixed
		return strings.TrimPrefix(trimmedURI, r2s.XName), nil
	} else if memberName == "@odata.type" {
		return r2s.Resource.ODataType, nil
	} else if memberName == "@odata.context" {
		// Query redis for the fields for this resource
		names, err := r2s.redis().SMembers(trimmedURI).Result()
		if err != nil {
			return "", err
		}

		propertyNames := []string{}
		for _, name := range names {
			// Remove any annotations
			if strings.Contains(name, "@") {
				continue
			}

			propertyNames = append(propertyNames, name)
		}

		sort.Strings(propertyNames)
		var fields string
		if len(propertyNames) < 25 {
			fields = strings.Join(propertyNames, ",")
		} else {
			fields = "*"
		}
		return r2s.Resource.ODataContext + "(" + fields + ")", nil
	}

	return "", errors.New("Unknown field name")
}

/* Quick, perhaps temporary, redis-only function for building an single action structure */
func (r2s *Redis2Interface) buildAction(keySpace string) (map[string]interface{}, error) {
	properties, err := r2s.redis().SMembers(keySpace).Result()
	if err != nil {
		return nil, errors.New("Failed to get action members from redis:" + keySpace)
	}
	payload := map[string]interface{}{}
	for _, name := range properties {
		key := keySpace + "/" + name
		rType, _ := r2s.redis().Type(key).Result()
		switch rType {
		case "list":
			result, _ := r2s.redis().LRange(key, 0, -1).Result()
			payload[name] = result
		case "string":
			result, _ := r2s.redis().Get(key).Result()
			payload[name] = strings.TrimPrefix(result, r2s.XName)
		default:
			return nil, errors.New("Unhandled datatype for:" + key)
		}
	}
	return payload, nil
}

// Returns the most appropriate result for a given key.
// If the result is stored in Redis it will return that, otherwise it will try to execute a script at the key's
// location on the filesystem and return that.
func (r2s *Redis2Interface) getValueForKey(key string) (string, error) {
	logFields := log.Fields{
		"key":       key,
		"r2s.XName": r2s.XName,
	}

	// Check Redis for exact keypath.
	simpleValue, err := r2s.redis().Get(key).Result()
	if err == nil {
		logFields["simpleValue"] = simpleValue
		log.WithFields(logFields).Trace("Got value from Redis")
		return simpleValue, nil
	}

	// Check to see if this is an OData key.
	simpleValue, err = r2s.generateOData(key)
	if err == nil {
		logFields["simpleValue"] = simpleValue
		log.WithFields(logFields).Trace("Got OData")
		return simpleValue, nil
	}

	// If none of the above worked then by this point we can be sure what we're looking for is not in Redis.
	// The very next thing we will try is to see if there is a file handler. We will always try this as for debugging
	// purposes it would be great to have the ability to change the behavior of this application by just popping
	// down a script in a well known location.

	// Read/Execute file
	buffer, err := r2s.RFD.DoFileHandler(OutputOp, key, nil, nil) // TODO Should maybe pass in an environment into there
	if err == nil {
		simpleValue = string(buffer)
		// Trim off new line
		simpleValue = strings.TrimRight(simpleValue, "\n")
		logFields["simpleValue"] = simpleValue
		log.WithFields(logFields).Trace("Got data from file handler")
		return simpleValue, nil
	}

	// The other thing we will always do is call a BackendHelper which could either be a real function or just a mock
	// no-op style call.
	for _, backendHelper := range r2s.RFD.BackendHelpers {
		var env map[string]string
		if r2s.XName != "" {
			// If the Host is set let's build up the environment variables.
			env, _ = backendHelper.GetEnvForXname(r2s.XName)
		}

		// Define a context with a timeout.
		timeout := 180 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		log.WithFields(log.Fields{"key": key, "xname": r2s.XName}).Debug("calling backendhelper")
		simpleValue, err = backendHelper.RunBackendHelper(ctx, key, nil, env)
		log.WithFields(log.Fields{"key": key, "xname": r2s.XName, "value": simpleValue, "err": err}).Debug("back from backendhelper")
		if err == nil {
			logFields["simpleValue"] = simpleValue
			log.WithFields(logFields).Trace("Got data from backend helper")
			return simpleValue, nil
		} else if err == backend_helpers.ErrBackendContinue {
			log.WithFields(logFields).Trace("Backend does not apply, going to next in line")
			continue
		} else if strings.HasPrefix(err.Error(), "unknown xname") {
			// TODO think about using a error.Is or multierror here
			log.WithFields(log.Fields{"key": key, "xname": r2s.XName}).Debug("unknown xname")
			return "", err
		}
	}

	detailedError := errors.New("Unable to find handler for key")
	log.WithFields(logFields).Error(detailedError.Error())
	return "", detailedError
}

/* Helper function that (if needed) will get a resource and build a paylaod
 * for a link that is set to autoexpand */
func (r2s *Redis2Interface) buildAutoExpandedLink(property *rfschema.Property, keySpace string) ([]interface{}, error) {
	// TODO Evaluate if auto-expanded links are always arrays
	resource := property.Link
	indices, err := r2s.redis().LRange(keySpace, 0, -1).Result()
	if err != nil {
		log.Warn("No redis value for link: " + keySpace)
		return nil, errors.New("No redis value for link: " + keySpace)
	}
	payload := make([]interface{}, len(indices))
	for i, iStr := range indices {
		memberKeySpace := keySpace + "/" + iStr
		member, _ := r2s.BuildStruct(resource.CollectionOf.Properties, memberKeySpace)
		payload[i] = member
	}

	log.Debug("Auto expanded array length: ", len(payload))
	return payload, nil
}

/* Helper function that for building links. Depending on whether the TODO */
func (r2s *Redis2Interface) buildLink(property *rfschema.Property, keySpace string) (interface{}, error) {
	if property.LinkAutoExpand {
		log.WithField("keySpace", keySpace).Trace("Building auto expanded link")
		link, err := r2s.buildAutoExpandedLink(property, keySpace)
		if err != nil {
			log.WithField("keySpace", keySpace).Warning("Unable to build auto expanded link")
			return nil, err
		}
		return link, nil
	}

	// TODO: Evaluate if it would be better to get this from prop (linkRef)
	// Ryan: linkRef contains the JSON reference to the linked schema, I'm not sure if it would work well
	// here
	var result string
	if property.IsNavLink {
		// Since this is a navigation link the link is the same as the key
		result = keySpace
	} else {
		// Access redis for value
		link := keySpace + "/" + "@odata.id"
		result, _ = r2s.redis().Get(link).Result()
	}

	// Make sure we don't present any xname prefixed links, they're not valid.
	result = strings.TrimPrefix(result, r2s.XName)

	link := map[string]interface{}{}
	link["@odata.id"] = result

	return link, nil
}

/* Helper function that will build an array from redis */
func (r2s *Redis2Interface) buildArray(property *rfschema.Property, keySpace string) ([]interface{}, error) {
	array := []interface{}{}
	var elements []string
	var err error

	// Get the members.
	elements, err = r2s.redis().LRange(keySpace, 0, -1).Result()
	if err != nil {
		return nil, err
	}

	elementProperty := property.ArrayOf
	for _, element := range elements {
		if isRedfishValueString(elementProperty.Type) {
			// The value is stored directly in the redis list
			array = append(array, element)
		} else if elementProperty.Type == rfschema.PropLink {
			key := keySpace + "/" + element
			link := key + "/" + "@odata.id"
			result, _ := r2s.redis().Get(link).Result()

			// Make sure we don't present any xname prefixed links, they're not valid.
			result = strings.TrimPrefix(result, r2s.XName)

			value := map[string]interface{}{}
			value["@odata.id"] = result
			array = append(array, value)
		} else {
			key := keySpace + "/" + element
			value, err := r2s.BuildStruct(elementProperty.Properties, key)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
	}

	return array, nil
}

// This function works by recursively building up the structure that ultimately will be marshaled into JSON and
// presented to the user. It does this by progressively moving its way down the property list (the `properties`
// argument) and keyspace generating the correct type (array, object, etc.) at each level.
func (r2s *Redis2Interface) BuildStruct(properties map[string]*rfschema.Property,
	baseKeySpace string) (map[string]interface{}, error) {
	var err error
	payload := map[string]interface{}{}
	var keys []string

	// First check the exact keySpace to see if it exists.
	keySpace := baseKeySpace
	keys, err = r2s.redis().SMembers(keySpace).Result()
	if err != nil || len(keys) == 0 {
		// Didn't find it, now check with the hostname prepended.
		if !strings.HasPrefix(baseKeySpace, r2s.XName) {
			keySpace = r2s.XName + baseKeySpace
		}

		keys, err = r2s.redis().SMembers(keySpace).Result()
		if err != nil {
			return nil, errors.New("No redis value for link: " + keySpace)
		}
	}

	for _, name := range keys {
		var simpleValue string
		key := keySpace + "/" + name
		property, found := properties[name]
		if found == false {
			err = errors.New("No Redfish property exists for field")
			log.WithFields(log.Fields{
				"name": name,
				"key":  key,
			}).Error(err.Error())
			return nil, err
		}

		/* complex types result in recursion, nothing is set here */
		if isRedfishValueString(property.Type) {
			simpleValue, err = r2s.getValueForKey(key)
			if err != nil {
				log.WithFields(log.Fields{
					"key": key,
					"err": err,
				}).Warning("Failed to get value for key")
				if strings.HasPrefix(err.Error(), "unknown xname") {
					return nil, err
				}
				// Set the field to null and continue on
				payload[name] = nil
				continue
			}
		}

		logFields := log.Fields{
			"keyspace":    keySpace,
			"name":        name,
			"simpleValue": simpleValue,
		}

		switch property.Type {
		case rfschema.PropAction:
			payload[name] = map[string]interface{}{}
			payload[name], err = r2s.buildAction(key)
			if err != nil {
				logFields["payload"] = payload
				log.WithFields(logFields).Error("Failed to build action")

				return nil, err
			}
		case rfschema.PropObject:
			payload[name], err = r2s.BuildStruct(property.Properties, key)
			if err != nil {
				logFields["payload"] = payload
				log.WithFields(logFields).Error("Failed to build object")

				return nil, err
			}
		case rfschema.PropArray:
			array, err := r2s.buildArray(property, key)
			if err != nil {
				logFields["property"] = property
				logFields["key"] = key
				log.WithFields(logFields).Error("Failed to build array")

				return nil, err
			}
			payload[name] = array
			payload[name+"@odata.count"] = len(array)
		case rfschema.PropLink:
			link, err := r2s.buildLink(property, key)
			if err != nil {
				logFields["property"] = property
				logFields["key"] = key
				log.WithFields(logFields).Error("Failed to build link")

				return nil, err
			}
			payload[name] = link

			// If the link is array add the @odata.count annotation
			if array, ok := link.([]interface{}); ok {
				payload[name+"@odata.count"] = len(array)
			}
		case rfschema.PropEnum:
			// TODO add enum value check
			fallthrough
		case rfschema.PropString:
			payload[name] = simpleValue
		case rfschema.PropNumber:
			if simpleValue == "" {
				// If the values is empty then we should treat the number as null. As we are always dealing with the string representation of the number, so we can safely assume that when we encounter a empty string that the value is null.
				payload[name] = nil
			} else {
				value, err := strconv.ParseFloat(simpleValue, 64)
				if err != nil {
					log.WithFields(logFields).Error("Failed to convert string to float")
					return nil, errors.New("failed to convert string to float")
				}
				payload[name] = value
			}
		case rfschema.PropInteger:
			value, err := strconv.ParseInt(simpleValue, 10, 64)
			if err != nil {
				log.WithFields(logFields).Error("Failed to convert string to integer")
				return nil, errors.New("failed to convert string to integer")
			}
			payload[name] = value
		case rfschema.PropBool:
			value, err := strconv.ParseBool(simpleValue)
			if err != nil {
				log.WithFields(logFields).Error("Failed to convert string to boolean")
				return nil, errors.New("failed to convert string to boolean")
			}
			payload[name] = value
		default:
			logFields["property.Type"] = property.Type
			log.WithFields(logFields).Error("Unknown property type")

			return nil, errors.New("unknown property type")
		}

		log.WithFields(log.Fields{
			"property.Type": property.Type,
			"value":         payload[name],
			"name":          name,
		}).Trace("Got value for property")
	}
	return payload, nil
}

/* This function will convert the given structure field/property value into its corresponsing value for the
 * intermediate data structure that is used to insert data into redis.
 *
 * Note: The bool value that is returned is used to signals that the intermediate value is null.
 */
func (rfd *RedfishDispatcher) structField2Redis(path []string, property *rfschema.Property, valueRaw interface{}, cb Interface2RedisCB) (interface{}, bool, error) {
	if property == nil {
		panic("Nil Property")
	}
	var result interface{}

	if valueRaw == nil && property.NullAllowed {
		allowed := true
		if cb != nil {
			allowed = cb(property, path, valueRaw, false)
		}
		return nil, allowed, nil
	}
	// TODO: What should happen if the value of field is null but the property does not allow null?

	switch property.Type {
	case rfschema.PropObject:
		obj, ok := valueRaw.(map[string]interface{})
		if !ok {
			return nil, false, errors.New("Unable to convert type for field: " + strings.Join(path, "/"))
		}
		objRedis, err := rfd.Interface2Redis(path, property.Properties, obj, cb)
		if err != nil {
			return nil, false, err
		}
		result = objRedis
	case rfschema.PropAction:
		// TODO
	case rfschema.PropEnum:
		// TODO check enum values
		fallthrough
	case rfschema.PropString:
		value, ok := valueRaw.(string)
		if !ok {
			return nil, false, errors.New("Unable to convert type for field: " + strings.Join(path, "/"))
		}
		result = value
	case rfschema.PropNumber:
		value, ok := valueRaw.(float64)
		if !ok {
			return nil, false, errors.New("Unable to convert type for field: " + strings.Join(path, "/"))
		}
		result = fmt.Sprint(value)
	case rfschema.PropInteger:
		valueFloat, ok := valueRaw.(float64)
		if !ok {
			return nil, false, errors.New("Unable to convert type for field: " + strings.Join(path, "/"))
		}
		value := int64(valueFloat)
		result = strconv.FormatInt(value, 10)
	case rfschema.PropBool:
		value, ok := valueRaw.(bool)
		if !ok {
			return nil, false, errors.New("Unable to convert type for field: " + strings.Join(path, "/"))
		}
		result = strconv.FormatBool(value)
	case rfschema.PropArray:
		array, ok := valueRaw.([]interface{})
		if !ok {
			return nil, false, errors.New("Unable to convert type for field: " + strings.Join(path, "/"))
		}
		resultArray, err := rfd.structArray2Redis(path, property, array, cb)
		if err != nil {
			return nil, false, err
		}
		result = resultArray
	case rfschema.PropLink:
		value := valueRaw.(map[string]interface{})
		if !property.IsNavLink {
			linkRaw := value["@odata.id"]
			result = map[string]interface{}{
				"@odata.id": linkRaw.(string),
			}
		}
	default:
		panic("Cannot handle type:" + property.Type.String())
	}

	if cb != nil && !cb(property, path, result, false) {
		log.Trace("Failed callback")
		return nil, false, nil
	}

	log.Trace("Result:", result)
	return result, false, nil
}

/* redisArray is used in the intermediate data structure that is used between Interface2Redis and RedisInsert/reddisPatch.
 * In our redis schema there are 2 ways a array can be represented.
 * The first way is a list of strings, where each element of the list is element of our array.
 * If the array is an array objects then each element of the list is an
 * unique id (such as 0, 1, etc..). The unique id can be used with the arrays key to get to the redis set that
 * describes the object. The object specified by can be located at {array key}/{Id}  */
type redisArray struct {
	isObject bool
	elements []interface{}
}

/* This function will populate a redisArray structure for an array from and Redfish resource or object.
 * If the element type of this array is can be represetned as a string then `elements` array in the redisArray will
 * contain a string value.
 * If the element type is an object then each element of the `elements` array will be an object.
 * */
func (rfd *RedfishDispatcher) structArray2Redis(path []string, arrayProperty *rfschema.Property, array []interface{}, cb Interface2RedisCB) (redisArray, error) {
	if arrayProperty == nil {
		panic("Nil Property")
	}

	property := arrayProperty.ArrayOf
	result := redisArray{}
	result.isObject = !isRedfishValueString(property.Type)

	for i, elementRaw := range array {
		elementPath := append(path, fmt.Sprintf("%d", i))
		element, _, err := rfd.structField2Redis(elementPath, property, elementRaw, cb)
		if err != nil {
			return redisArray{}, err
		}
		result.elements = append(result.elements, element)
	}

	return result, nil
}

/* The Interface2RedisCB callback function is used by Interface2Redis, structArray2Redis, and structField2Redis
 * to determine if a property should be added to the intermediate data structure.
 *
 * If the property is known then the `property` argument will contain a valid reference to a schema property, otherwise
 * if is is unknown the schema property will be nil and the `unknown` argument will be true
 *
 * The `path` argument is a array that contains the path to this field in the original structure being converted, where
 * each element is the name of a field that needed to be traversed to this property.
 * */
type Interface2RedisCB func(property *rfschema.Property, path []string, value interface{}, unknown bool) bool

/* This function will convert an Resource/object into an intermediate data structure that can be easily inserted into
 * redis.
 *
 * This data structure is a map where names of properties are keys:
 * - All "simple values" are converted into strings
 * - Arrays are represented by the redisArray structure
 * - Objects are maps
 *
 * The callback function has a few purposes.
 * The main purpose of the callback function is when a property is encountered it will be ran. If the call back
 * returns true then the property will be added to the data structure, otherwise if false the property will be ignored.
 * The other purpose of the callback function is to log when unknown properties are encountered
 */
func (rfd *RedfishDispatcher) Interface2Redis(path []string, properties map[string]*rfschema.Property, object map[string]interface{}, cb Interface2RedisCB) (map[string]interface{}, error) {
	result := map[string]interface{}{}

	for name, valueRaw := range object {
		propertyPath := append(path, name)
		property, ok := properties[name]
		if !ok {
			if cb == nil {
				return nil, errors.New("Unknown property: " + name)
			}
			// Notify the call back an unknown property was encountered
			cb(nil, append(path, name), valueRaw, true)
			continue
		}

		resultField, isNull, err := rfd.structField2Redis(propertyPath, property, valueRaw, cb)
		if err != nil {
			return nil, err
		}

		if resultField != nil || isNull {
			log.Trace("Adding field", name, ":", resultField)
			result[name] = resultField
		}
	}

	return result, nil
}

/* RedisInsert is used to insert the intermediate data structure generated by Interface2Redis into redis
 * When RedisInsert encounters a nil value no value will be added in redis, but the object set in redis
 * will specify what that it is a member.
 * TODO: Should a sentinel value be used instead of nil to represent that the value is dispatched? */
func (rfd *RedfishDispatcher) RedisInsert(valueRaw interface{}, key string) error {
	switch valueRaw.(type) {
	case string:
		// Simple value
		value := valueRaw.(string)
		log.Trace("STRING", key, value)
		err := rfd.redis.Set(key, value, 0).Err()
		if err != nil {
			return err
		}
	case redisArray:
		// Array
		array := valueRaw.(redisArray)

		if array.isObject {
			// Objects
			for i, elementRaw := range array.elements {
				// Add object to key to array
				log.Trace("RPUSH", key, i)
				err := rfd.redis.RPush(key, i).Err()
				if err != nil {
					return err
				}

				// Force conversions to make sure that this is a object
				element := elementRaw.(map[string]interface{})
				objKey := key + "/" + fmt.Sprint(i)
				rfd.RedisInsert(element, objKey)
			}
		} else {
			// Simple string values
			log.Trace("RPUSH", key, fmt.Sprint(array.elements...))
			err := rfd.redis.RPush(key, array.elements...).Err()
			if err != nil {
				return err
			}
		}
	case map[string]interface{}:
		// Object
		data := valueRaw.(map[string]interface{})
		names := []interface{}{}
		for name := range data {
			names = append(names, name)
		}

		for name, propertyRaw := range data {
			propertyKeySpace := key + "/" + name
			// Insert property value as normal
			rfd.RedisInsert(propertyRaw, propertyKeySpace)
		}

		// Create the object set after all of its children keys are in redis.
		log.Debug("Adding fields to ", key)
		err := rfd.redis.SAdd(key, names...).Err()
		if err != nil {
			panic(err)
		}

	case nil:
		// Ignore the value
	default:
		panic("Unable to handle type of: " + fmt.Sprintf("%T", valueRaw))
	}

	return nil
}

/* redisPatch uses intermediate data structure generated by Interface2Redis and updates/patches the data stored in redis.
 * TODO: Updating of arrays is not implemented */
func (rfd *RedfishDispatcher) RedisPatch(valueRaw interface{}, key string) error {
	switch valueRaw.(type) {
	case string:
		// Simple value
		value := valueRaw.(string)
		log.Trace("STRING", key, value)
		err := rfd.redis.Set(key, value, 0).Err()
		if err != nil {
			return err
		}
	case redisArray:
		panic("PATCH operations are unsupported on arrays")
	case map[string]interface{}:
		// Object
		data := valueRaw.(map[string]interface{})
		for name, propertyRaw := range data {
			propertyKeySpace := key + "/" + name

			if propertyRaw == nil {
				// Remove key from containing object
				err := rfd.redis.SRem(key, name).Err()
				if err != nil {
					panic(err)
				}

				// Remove key
				keyType, err := rfd.redis.Type(propertyKeySpace).Result()
				if err != nil {
					panic(err)
				}
				switch keyType {
				case "string":
					log.Trace("DEL", propertyKeySpace)
					err = rfd.redis.Del(propertyKeySpace).Err()
					if err != nil {
						panic(err)
					}
				case "set":
					rfd.redisRemoveObject(nil, propertyKeySpace)
				case "list":
					// TODO
					log.Debug("Unable to handle removing list durning patch right now")
				}
			} else {
				// Insert property value as normal
				rfd.RedisPatch(propertyRaw, propertyKeySpace)
				// Add the property name to the set specifing fields for this object. Since this is a set the duplicate
				// name will be ignored
				err := rfd.redis.SAdd(key, name).Err()
				if err != nil {
					panic(err)
				}
			}
		}
	case nil:
		// TODO Ignore for now
	default:
		panic("Unable to handle type of: " + fmt.Sprintf("%T", valueRaw))
	}

	return nil
}

/* redisRemoveResource will remove a resource at the given the given uri/key */
func (rfd *RedfishDispatcher) RedisRemoveResource(resource *rfschema.Resource, uri string) {
	rfd.redisRemoveObject(resource.Properties, uri)
}

/* redisRemoveField will attempt to remove this key from redis.
 * If the type of this field/key is a string, then the key will be deleted.
 * If the type is a set (meaning object) then redisRemoveObject will be called
 * If the type is a list then redisRemoveArray will be called */
func (rfd *RedfishDispatcher) redisRemoveField(property *rfschema.Property, keySpace string) {
	keyType, err := rfd.redis.Type(keySpace).Result()
	if err != nil {
		panic(err)
	}

	switch keyType {
	case "string":
		// The key can be removed outright
		log.Trace("DEL", keySpace)
		err = rfd.redis.Del(keySpace).Err()
		if err != nil {
			panic(err)
		}
	case "set":
		var properties map[string]*rfschema.Property
		if property != nil {
			// Do not recurse into a different resource's keys
			// TODO: is this an expected behavior? Or should resources that decsend from this one be removed also?
			// If a resource is removed that contains navigation links to another resource, should the linked resources
			// also be removed as they can be no longer be accessed?
			//
			// Also this behavior is beneficial for PUT operations, as it replaces the resource in place (if this
			//  function) is going to be used to help make a PUT operation by removing all the keys first. TBD
			if property.Type == rfschema.PropLink && property.IsNavLink {
				return
			}
			properties = property.Properties
		}
		rfd.redisRemoveObject(properties, keySpace)
	case "list":
		rfd.redisRemoveArray(property, keySpace)
	}
}

/* redisRemoveObject will remove all of the keys that descend from this this object. Removing a object will also remove
 * all simple values, arrays, and child objects that it contains. */
func (rfd *RedfishDispatcher) redisRemoveObject(properties map[string]*rfschema.Property, keySpace string) {
	// Remove all members of this object
	members, err := rfd.redis.SMembers(keySpace).Result()
	if err != nil {
		panic(err)
	}

	// Remove the set containing object members
	log.Debug("Removing set: ", keySpace)
	log.Trace("DEL", keySpace)
	err = rfd.redis.Del(keySpace).Err()
	if err != nil {
		panic(err)
	}

	for _, name := range members {
		// If there exists an schema property for this key get it, otherwise pass nil to removeField
		schemaProperty := properties[name]
		rfd.redisRemoveField(schemaProperty, keySpace+"/"+name)
	}
}

/* redisRemoveArray will remove an array from redis. */
func (rfd *RedfishDispatcher) redisRemoveArray(arrayProperty *rfschema.Property, keySpace string) {
	if arrayProperty != nil && !isRedfishValueString(arrayProperty.ArrayOf.Type) {
		// Query for array elements
		elements, err := rfd.redis.LRange(keySpace, 0, -1).Result()
		if err != nil {
			panic(err)
		}

		for _, element := range elements {
			// If the element is not a object than Properties will be a empty map
			rfd.redisRemoveObject(arrayProperty.ArrayOf.Properties, keySpace+"/"+element)
		}

	}
	// Need to remove list
	log.Trace("DEL", keySpace)
	rfd.redis.Del(keySpace)
	err := rfd.redis.Del(keySpace).Err()
	if err != nil {
		panic(err)
	}
}
