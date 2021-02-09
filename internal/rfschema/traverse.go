// MIT License
//
// (C) Copyright [2021] Hewlett Packard Enterprise Development LP
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
