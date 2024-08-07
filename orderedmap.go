package img2ansi

import (
	"sync"
)

// OrderedMap represents an ordered map data structure
type OrderedMap struct {
	keys   []interface{}
	values map[interface{}]interface{}
	mu     sync.RWMutex
}

// New creates a new OrderedMap
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{
		keys:   make([]interface{}, 0),
		values: make(map[interface{}]interface{}),
	}
}

// Set adds a Key-Value pair to the map
func (om *OrderedMap) Set(key, value interface{}) {
	om.mu.Lock()
	defer om.mu.Unlock()

	if _, exists := om.values[key]; !exists {
		om.keys = append(om.keys, key)
	}
	om.values[key] = value
}

// Get retrieves a Value from the map by Key
func (om *OrderedMap) Get(key interface{}) (interface{}, bool) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	val, exists := om.values[key]
	return val, exists
}

// Delete removes a Key-Value pair from the map
func (om *OrderedMap) Delete(key interface{}) {
	om.mu.Lock()
	defer om.mu.Unlock()

	if _, exists := om.values[key]; exists {
		delete(om.values, key)
		for i, k := range om.keys {
			if k == key {
				om.keys = append(om.keys[:i], om.keys[i+1:]...)
				break
			}
		}
	}
}

// Keys returns a slice of keys in the order they were inserted
func (om *OrderedMap) Keys() []interface{} {
	om.mu.RLock()
	defer om.mu.RUnlock()

	return append([]interface{}{}, om.keys...)
}

// Iterate calls the provided function for each Key-Value pair in order
func (om *OrderedMap) Iterate(f func(key, value interface{})) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	for _, k := range om.keys {
		f(k, om.values[k])
	}
}

// Len returns the number of elements in the map
func (om *OrderedMap) Len() int {
	om.mu.RLock()
	defer om.mu.RUnlock()

	return len(om.keys)
}
