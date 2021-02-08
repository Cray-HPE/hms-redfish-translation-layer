// MIT License
//
// (C) Copyright [2018-2021] Hewlett Packard Enterprise Development LP
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
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Tree hold the root of the Redfish Schema tree. Which is the ServiceRoot.
type Tree struct {
	Version     Version
	ServiceRoot *Resource

	SchemaPool *SchemaPool
}

// CreateTree will parse the schema directory to create the Schema Tree
func CreateTree(config *Config, version Version) (*Tree, error) {
	// Stage 0 - Load Schemas
	pool := NewSchemaPool(version)
	err := pool.ReadDir(config.SchemaDir)
	if err != nil {
		return nil, err
	}

	if config.OemMappings != "" {
		err := pool.AddOemExtensions(config.OemMappings, config.OemExtensionDir)
		if err != nil {
			return nil, err
		}
	}

	// Stage 1 - Parsing
	collections, resources := parseSchemas(pool, version)

	// s1 := resources["Storage"]
	// sc1 := s1.Properties["StorageControllers"]
	// fmt.Println("StorageControllers:", sc1.Type)

	// if sc1.ArrayOf.Type == PropLink {
	// 	panic("Storage controllers is a link")
	// }

	// Stage 2 - Linking
	eCollections, eResources := detectEmbeddedCollections(resources)
	for name, collection := range eCollections {
		collections[name] = collection
	}
	for name, resource := range eResources {
		resources[name] = resource
	}

	// s := resources["Storage"]
	// sc := s.Properties["StorageControllers"]
	// fmt.Println("StorageControllers:", sc.Type)

	// if sc.ArrayOf.Type == PropLink {
	// 	panic("Storage controllers is a link")
	// }

	detectNavigationLinks(resources)
	detectNavigationArrays(resources)

	// TODO: Need to link non navigational links

	return linkDFS(pool, collections, resources), nil
}

// parseSchemas will attempt to parse all collections and resources
func parseSchemas(pool *SchemaPool, version Version) (map[string]*Resource, map[string]*Resource) {
	collections := make(map[string]*Resource)
	resources := make(map[string]*Resource)

	versionMapping, ok := GetVersionMapper().GetMapping(version)
	if !ok {
		panic(version.String() + " does not exist")
	}

	for file, schema := range pool.Pool {
		debugPrintln(0, "Parsing:", file)
		switch {
		case schema.IsCollection():
			// Skip collections that do not exist in this version of the schema
			if !versionMapping.AllowedCollections[schema.FileName] {
				continue
			}
			collection, name := ParseCollection(schema)
			collections[name] = collection
			debugPrintln(0, "[*] Collection", name)
		case schema.IsResource():
			// Skip resources that do not exist in this version of the schema
			if _, ok := versionMapping.Resources[schema.FileName]; !ok {
				continue
			}
			resource, name := ParseResource(schema)
			resources[name] = resource
			debugPrintln(0, "[*] Resource", name)
		default:
			debugPrintln(0, "[ ] Ignoring", file)
		}
	}

	return collections, resources
}

func detectNavigationLinks(resources map[string]*Resource) {
	// If a resource has a top level property that is a link, then it is also a navigation link in the tree.
	// It is being assumed that if a resource has a Property that is a link then a instance of that resource
	// will have its own instance of the resource that it is linking too.
	for _, resource := range resources {
		for name, property := range resource.Properties {
			if property.Type == PropLink {
				property.IsNavLink = true
				resource.Properties[name] = property
			} else if name != "Links" && property.Type == PropObject {
				// Descend into each non-Links object looking for navigation links
				for childName, childProperty := range property.Properties {
					if childProperty.Type == PropLink {
						childProperty.IsNavLink = true
						property.Properties[childName] = childProperty
						resource.Properties[name] = property
					}
				}
			}
		}
	}
}

func detectNavigationArrays(resources map[string]*Resource) {
	// If a resource has a top level property that is an array of links, then all of these navigation links
	// Such as the the Redundancy field in Fan that is an collection of links to Redundancies.

	// In the CSDL the navigation property would look something like the following
	// <NavigationProperty Name="Redundancy" Type="Collection(Redundancy.Redundancy)">
	//  <Annotation Term="OData.Description" String="This structure is used to show redundancy for fans.  The Component ids will reference the members of the redundancy groups."/>
	//  <Annotation Term="OData.LongDescription" String="The values of the properties in this array shall be used to show redundancy for fans and other elements in this resource.  The use of IDs within these arrays shall reference the members of the redundancy groups."/>
	//  <Annotation Term="OData.AutoExpandReferences"/>
	// </NavigationProperty>
	// BUG: This will not take in to consideration if the annotation OData.AutoExpand is used for a collection.
	// TODO: detectNavigationArrays and detectEmbeddedCollections should be updated to respect the new "autoExpand" property that is
	// present in the 2018.2 JSON Schema release

	for _, resource := range resources {
		if resource.Name == "ResourceBlock" {
			//TODO: Ignore for now
			continue
		}
		for name, property := range resource.Properties {
			if property.Type == PropArray && property.ArrayOf.Type == PropLink && property.ArrayOf.linkRef != "" {
				debugPrintln(0, "Navigation Array: ", resource.Name, name)
				property.IsNavArray = true
				resource.Properties[name] = property
			}
		}
	}

}

func detectEmbeddedCollections(parentResources map[string]*Resource) (map[string]*Resource, map[string]*Resource) {
	// If a resource has a top level property that is an array, and if the array is of an object that contains a 'MemberId' then this is a collection
	// This would include things such as the array of Fans in the Thermal entity.

	// In the CSDL the navigation property would include an OData.AutoExpand annotation.
	// <NavigationProperty Name="Fans" Type="Collection(Thermal.v1_0_0.Fan)" ContainsTarget="true">
	// 	<Annotation Term="OData.Permissions" EnumMember="OData.Permission/ReadWrite"/>
	// 	<Annotation Term="OData.Description" String="This is the definition for fans."/>
	// 	<Annotation Term="OData.LongDescription" String="These properties shall be the definition for fans for a Redfish implementation."/>
	// 	<Annotation Term="OData.AutoExpand"/>
	// </NavigationProperty>
	// TODO/BUG: This only handles when the resource/entity that is being link is represented as a object
	collections := make(map[string]*Resource)
	resources := make(map[string]*Resource)
	for _, parentResource := range parentResources {
		for name, property := range parentResource.Properties {
			if property.Type == PropArray {
				if _, ok := property.ArrayOf.Properties["MemberId"]; ok {
					debugPrintln(0, "Embedded Collection:", parentResource.Name, name)
					collection, resource := convertArrayToCollection(parentResource, name)
					collections[collection.Name] = collection
					resources[resource.Name] = resource
				}
			}
		}
	}
	return collections, resources
}

func convertArrayToCollection(parentResource *Resource, propertyName string) (*Resource, *Resource) {
	// NOTE: It appears that on difference Redfish impmentations that you can either access say a fan directly
	// by its OData id or you can't Like in AMIs Redfish front end.
	property := parentResource.Properties[propertyName]

	if property.arrayRefString == nil {
		panic("Array does not have reference string")
	}

	refString := *property.arrayRefString

	resourceType := extractLinkedResourceType(refString)
	odataType := "#" + parentResource.Name + "." + parentResource.VersionString + "." + resourceType

	resource := &Resource{
		Name:         resourceType,
		Properties:   property.ArrayOf.Properties,
		ODataContext: parentResource.ODataContext, // Same context
		ODataType:    odataType,
		// RequiredProperties: // Note: The JSON Schema does not define any required properties for Object Properties
	}

	if property.AdditionalProperties != nil {
		resource.AdditionalProperties = *property.ArrayOf.AdditionalProperties
	}

	if property.PatternProperties != nil {
		resource.PatternProperties = property.PatternProperties
	}

	collection := &Resource{
		Name:         resourceType + "Collection",
		IsCollection: true,
		// CollectionOf:       resource,
		EmbeddedCollection: true,
		// Since the collection is embedded there are no OData Context or OData Type associated with it
		ODataContext: "",
		ODataType:    "",
	}

	property.Type = PropLink
	// property.Link = collection
	property.LinkAutoExpand = true
	property.IsNavLink = true

	// This isn't a real reference, but its enough to fool linkDFS
	property.linkRef = "#/definitions/" + collection.Name

	// Update Property
	parentResource.Properties[propertyName] = property

	return collection, resource
}

func countReferences(tree *Tree) {
	onResourceVisit := func(resource *Resource) {
		if resource.IsCollection {
			resource.CollectionOf.ReferenceCount++

			resource.CollectionOf.Linkers = append(resource.CollectionOf.Linkers, resource)
		} else {
			// Count references from nav links
			for _, property := range resource.Properties {
				if property.Type == PropLink && property.IsNavLink {
					property.Link.ReferenceCount++
					property.Link.Linkers = append(property.Link.Linkers, resource)
				}
			}
		}
	}

	TraverseTree(tree, onResourceVisit)
}

func linkNavProperty(tree *Tree, allResources map[string]*Resource, resource *Resource, name string, property *Property) {
	log.WithFields(log.Fields{
		"name":             name,
		"resource.Name":    resource.Name,
		"property.linkRef": property.linkRef,
	}).Trace("Linking Navigation property")
	linkedResourceType := extractLinkedResourceType(property.linkRef)
	if linkedResourceType == "" {
		panic("Could not extract linked resource type:" + property.linkRef)
	}

	linkedResource, ok := allResources[linkedResourceType]
	if !ok {
		// If this fails check first to make sure the JSON file is listed in the AllowedCollections
		// section of the mappings file you're using!!!
		log.WithFields(log.Fields{
			"name":               name,
			"resource.ODataType": resource.ODataType,
			"linkedResourceType": linkedResourceType,
		}).Panic("Property requested in resource does not exist in linked resource")
	}

	if property.Link != nil {
		panic("Link is not nil")
	}

	// Make sure that this is not a cycle
	// TODO: this could be more gracefully handled then just breaking the cycle
	mapping, found := GetVersionMapper().GetMapping(tree.Version)
	if !found {
		panic("Version mapping is not present")
	}
	if cycle, ok := mapping.KnownCycles[resource.Name]; ok {
		debugPrintln(0, "Resource: ", resource.Name)
		if cycle.SourceField == name && cycle.DestinationResource == linkedResource.Name {
			// There is a cycle. The only way to handle this at the moment is to ignore the link
			// TODO: For now we are adding a "CycleDetected" resource to signal that this occurred
			debugPrintln(0, "Cycle Detected at "+resource.Name+":"+name+" to "+linkedResource.Name)
			desc := "A cycle was detected when linking the schema tree"
			property.Link = &Resource{
				Name:        "CycleDetected",
				Description: &desc,
			}
			return
		}
	}

	if resource.Name == linkedResource.Name {
		panic("Cycle!")
	}

	// Create a copy of the resource
	linkedResource.InstanceID++
	property.Link = copyResource(linkedResource)

	resource.Properties[name] = property

	if property.Link == nil {
		panic("Nil Link")
	}
}

func linkDFS(pool *SchemaPool, collections, resources map[string]*Resource) *Tree {
	// When linking in DFS mode we need to create a new tree that is not connected in
	// any way to the schema pool, because each schema is going to be potentially resused

	allResources := make(map[string]*Resource)
	for name, resource := range collections {
		allResources[name] = resource
	}
	for name, resource := range resources {
		allResources[name] = resource
	}
	if len(allResources) != len(resources)+len(collections) {
		panic("Size mismatch")
	}

	// Sanity check
	for _, r := range allResources {
		checkNilLinks(r)
	}

	tree := &Tree{}
	tree.Version = pool.Version
	tree.SchemaPool = pool
	// Get the service root
	serviceRoot, ok := resources["ServiceRoot"]
	if !ok {
		panic("Could not get service root")
	}

	// Create Copy of Service Root
	tree.ServiceRoot = copyResource(serviceRoot)

	resourcesToVisit := ResourceStack{}
	visited := map[*Resource]bool{}

	// Start at service root
	// pathsToVisit.Push("Root")
	resourcesToVisit.Push(tree.ServiceRoot)

	for !resourcesToVisit.Empty() {
		resource := resourcesToVisit.Pop()

		// Sanity check
		for _, r := range allResources {
			checkNilLinks(r)
		}

		if !visited[resource] {
			visited[resource] = true
			debugPrintln(0, "Visiting: ", resource.Name)

			if resource.IsCollection {
				// Handle collections

				ok := strings.HasSuffix(resource.Name, "Collection")
				if !ok {
					panic("Does not have 'Collection' as a suffix: " + resource.Name)
				}

				resourceName := strings.TrimSuffix(resource.Name, "Collection")

				collectionOf, ok := resources[resourceName]
				if !ok {
					log.WithFields(log.Fields{
						"resourceName": resourceName,
					}).Panic("Does not have resource")
				}

				collectionOf.InstanceID++
				resource.CollectionOf = copyResource(collectionOf)

				// Add to resources to visit
				resourcesToVisit.Push(resource.CollectionOf)

			} else {
				for name, property := range resource.Properties {
					// Link Navigation links
					// If linkRef is empty, then it is a generic Link, that can point to anything
					// Note: At this stage the property should not be linked
					// TODO: the code that does interacts with the linkref in the 2 cases below could be merged together
					// somehow

					if property.Type == PropLink && property.IsNavLink && property.Link == nil && property.linkRef != "" {
						linkNavProperty(tree, allResources, resource, name, property)

						if property.Link == nil {
							panic("property link is nil")
						}

						// Add to resources to visit
						resourcesToVisit.Push(property.Link)

						debugPrintln(0, "Nav Link: "+resource.Name+":"+name)
					} else if name != "Links" && property.Type == PropObject {
						log.WithFields(log.Fields{
							"name": name,
						}).Trace("Descending into object looking for navigation links")

						// Decscend into child object looking for navigation links
						for childName, childProperty := range property.Properties {
							isNavLink := childProperty.Type == PropLink && childProperty.IsNavLink
							notYetLinked := property.Link == nil && property.linkRef != ""
							if !isNavLink && !notYetLinked {
								log.WithFields(log.Fields{
									"chidName":     childName,
									"name":         name,
									"isNavLink":    isNavLink,
									"notYetLinked": notYetLinked,
								}).Trace("Skipping child property")

								continue
							}

							linkNavProperty(tree, allResources, resource, childName, childProperty)
							if childProperty.Link == nil {
								panic("Child property link is nil")
							}

							// Add to resources to visit
							resourcesToVisit.Push(childProperty.Link)

							debugPrintln(0, "Nav Link: "+resource.Name+":"+name)
						}
					} else if property.Type == PropArray && property.IsNavArray && property.ArrayOf.linkRef != "" {
						debugPrintln(0, property.ArrayOf.linkRef)
						linkedResourceType := extractLinkedResourceType(property.ArrayOf.linkRef)
						if linkedResourceType == "" {
							panic("Could not extract linked resource type:" + property.ArrayOf.linkRef)
						}

						linkedResource, ok := allResources[linkedResourceType]
						if !ok {
							panic("Does not have resource: " + linkedResourceType)
						}

						if property.ArrayOf.Link != nil {
							panic("Link is not nil")
						}

						debugPrintln(0, "Array Linking: "+resource.Name+":"+name+"->"+linkedResource.Name)

						if resource.Name == linkedResource.Name {
							panic("Cycle!")
						}
						// Create a copy of the resource
						linkedResource.InstanceID++
						property.ArrayOf.Link = copyResource(linkedResource)
						resource.Properties[name] = property

						if property.ArrayOf.Link == nil {
							panic("Link is not nil")
						}

						// Add resource to visit
						resourcesToVisit.Push(property.ArrayOf.Link)

						debugPrintln(0, "Nav Array Link: "+resource.Name+":"+name)
					}
				}
			}
		}
	}

	// countReferences(tree)
	checkLinks(visited)

	consistencyCheck(tree, allResources)

	return tree
}

func consistencyCheck(tree *Tree, allResources map[string]*Resource) {

	// Check to see if nav property has a link
	linkVisit := func(resource *Resource) {
		// Ensure all nav links have a link
		for name, prop := range resource.Properties {
			if prop.Type == PropLink && prop.IsNavLink && prop.Link == nil {
				panic("Link is nil: " + resource.Name + "->" + name)
			}
		}
	}
	TraverseTree(tree, linkVisit)

	// Check to see if all resource have 1 or less references
	countReferences(tree)
	refVisit := func(resource *Resource) {
		// Check references
		if resource.ReferenceCount > 1 {
			// If a resource has more than 1 reference then this is not a tree
			panic("Resource has too many references")
		}
	}
	TraverseTree(tree, refVisit)
}

func checkNilLinks(resource *Resource) {
	for name, p := range resource.Properties {
		if p.Type == PropLink && p.IsNavLink {
			if p.Link != nil {
				panic(name + " Should be nil")
			}
		}
	}
}

func checkLinks(resources map[*Resource]bool) {
	for resource := range resources {
		for name, prop := range resource.Properties {
			if prop.Type == PropLink && prop.IsNavLink && prop.Link == nil {
				panic("Nil Nav Link: " + resource.Name + ":" + name)
			} else if prop.Type == PropArray && prop.IsNavArray && prop.ArrayOf.Link == nil {
				panic("Nil Nav Array Link: " + resource.Name + ":" + name)
			}
		}
	}
}

func extractLinkedResourceType(linkRef string) string {
	// Extract the name of the definition
	// Ex: http://redfish.dmtf.org/schemas/v1/{SomeResource}.json#/definitions\{The thing we want}
	if strings.HasPrefix(linkRef, "http://redfish.dmtf.org/schemas/v1/") {
		r := `^http:\/\/redfish\.dmtf\.org\/schemas\/v1\/[A-Z][A-Za-z]+\.json#\/definitions\/([A-Z][A-Za-z]+)`
		return regexp.MustCompile(r).ReplaceAllString(linkRef, "${1}")
	} else if strings.HasPrefix(linkRef, "#/definitions/") {
		return strings.TrimPrefix(linkRef, "#/definitions/")
	}
	panic("Bad format for link reference: " + linkRef)
}
