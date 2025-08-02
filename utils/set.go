// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"sync"
)

// Set implements a generic set data structure
type Set[T comparable] struct {
	items map[T]struct{}
	mu    sync.RWMutex
}

// NewSet creates a new set with optional initial capacity
func NewSet[T comparable](capacity ...int) GenericSet[T] {
	cap := 0
	if len(capacity) > 0 {
		cap = capacity[0]
	}
	return &Set[T]{
		items: make(map[T]struct{}, cap),
	}
}

// Add adds items to the set
func (s *Set[T]) Add(items ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for _, item := range items {
		s.items[item] = struct{}{}
	}
}

// Contains checks if an item is in the set
func (s *Set[T]) Contains(item T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	_, exists := s.items[item]
	return exists
}

// Remove removes an item from the set
func (s *Set[T]) Remove(item T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	delete(s.items, item)
}

// Clear clears all items
func (s *Set[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.items = make(map[T]struct{})
}

// Len returns the number of items
func (s *Set[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return len(s.items)
}

// List returns all items as a slice
func (s *Set[T]) List() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	result := make([]T, 0, len(s.items))
	for item := range s.items {
		result = append(result, item)
	}
	return result
}