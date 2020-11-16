package rfschema

import (
	"errors"
)

// ResourceStack is a Stack that holds strings
// This implementation does not have a set max size
type ResourceStack struct {
	items []*Resource // Newest item at beginning
}

// Push a string on the stack
func (s *ResourceStack) Push(resource *Resource) {
	// Prepend the new string the the front
	// This method bellow may not be the fastest method
	s.items = append([]*Resource{resource}, s.items...)
}

// Pop a string off the stack
func (s *ResourceStack) Pop() *Resource {
	if len(s.items) == 0 {
		panic(errors.New("No items in stack"))
	}

	resource := s.items[0]
	s.items = s.items[1:]
	return resource
}

// Empty determines if the stack is empty
func (s *ResourceStack) Empty() bool {
	return len(s.items) == 0
}

// OnResourceVisit defines callback function signature TraverseTree when a resource is visited
type OnResourceVisit func(*Resource)

// TraverseTree will traverse the schema tree and visit every resource present.
// When a resource is visited the OnResourceVisit callback will be ran
func TraverseTree(tree *Tree, onResourceVisit OnResourceVisit) {
	s := ResourceStack{}
	s.Push(tree.ServiceRoot)

	discoveredResources := make(map[*Resource]bool) //Set

	for !s.Empty() {
		resource := s.Pop()
		if !discoveredResources[resource] {
			discoveredResources[resource] = true

			onResourceVisit(resource)

			if resource.IsCollection {
				s.Push(resource.CollectionOf)
			} else {
				for name, prop := range resource.Properties {
					if prop.Type == PropLink && prop.IsNavLink {
						if prop.Link == nil {
							panic("Link does not exist: " + name)
						}
						s.Push(prop.Link)
					}
				}
			}
		}
	}
}
