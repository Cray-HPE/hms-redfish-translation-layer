// Copyright 2018-2020 Hewlett Packard Enterprise Development LP
package rfschema

import (
	"strings"

	"github.com/sirupsen/logrus"
)

func buildPath(tokens []string) string {
	return "/redfish/v1/" + strings.Join(tokens, "/")
}

// CollectionCB will be called to check to see if the given instance is apart of the collection
type CollectionCB func(collection *Resource, collectionPath string, instance string) /*Valid Instance*/ bool

// WalkTree will take the given URI path and walk the tree to reach the resource (and property if applicable)
// specified by this this URI
func WalkTree(tree *Tree, collectionCB CollectionCB, path string) (*Resource, *Property) {
	logCtx := logrus.WithField("path", path)

	// Break apart the path into tokens
	if !strings.HasPrefix(path, "/redfish/v1") {
		return nil, nil
	}
	path = strings.TrimPrefix(path, "/redfish/v1")
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	tokens := strings.Split(path, "/")
	logCtx.WithField("tokens", tokens).Trace("Path Tokens")

	// Special Case: If tokens is empty then we are pointing to the service root
	if len(tokens) == 1 && len(tokens[0]) == 0 {
		return tree.ServiceRoot, nil
	}

	// Now walk the tree using the tokens
	isWalkingObject := false
	currentResource := tree.ServiceRoot
	var currentProperty *Property
	for i, token := range tokens {
		var pType string
		if currentProperty != nil {
			pType = currentProperty.Type.String()
		}

		logCtx.WithFields(logrus.Fields{
			"resourceName":         currentResource.Name,
			"token":                token,
			"isWalkingObject":      isWalkingObject,
			"currentProperty.Type": pType,
		}).Trace("Visiting Resource")

		if currentResource.IsCollection {
			collectionPath := buildPath(tokens[0:i]) // Remove tail token
			if collectionCB(currentResource, collectionPath, token) {
				// Valid Instance
				currentResource = currentResource.CollectionOf
			} else {
				// Invalid instance
				logCtx.WithField("collectionPath", collectionPath).Trace("Invalid instance")

				return nil, nil
			}
		} else {
			var ok bool
			if !isWalkingObject {
				currentProperty, ok = currentResource.Properties[token]
			} else {
				// Sean: If the last encounter token was not a link but is an object,
				// which can be addressed via POST operations, then check if
				// token is a valid object property
				// Ryan: Before overwriting currentProperty check to see if the current token is a property
				// of the object.
				if propProperty, present := currentProperty.Properties[token]; present {
					currentProperty = propProperty
					ok = true
				} else {
					// Try again, because actions names in URIs do not contain the '#' symbol
					actionToken := "#" + token
					currentProperty, ok = currentProperty.Properties[actionToken]
				}
			}

			if !ok {
				// There is no tree nav link to check as the the property does not exist
				return nil, nil
			}

			if currentProperty.Type == PropLink && currentProperty.IsNavLink {
				if currentProperty.Link == nil {
					panic("Found nil link while walking")
				}

				// Valid navigation link/property
				currentResource = currentProperty.Link
				isWalkingObject = false
			} else if currentProperty.Type == PropObject ||
				currentProperty.Type == PropAction ||
				currentProperty.Type == PropEnum ||
				currentProperty.Type == PropString ||
				currentProperty.Type == PropInteger ||
				currentProperty.Type == PropNumber ||
				currentProperty.Type == PropLink {
				// Valid object for traversal detected
				isWalkingObject = true
			} else {
				// This property is not a link or object
				return nil, nil
			}
		}
	}

	logrus.WithFields(logrus.Fields{
		"resourceName":    currentResource.Name,
		"isWalkingObject": isWalkingObject,
	}).Trace("Ended on resource")

	if isWalkingObject {
		return currentResource, currentProperty
	}
	return currentResource, nil
}
