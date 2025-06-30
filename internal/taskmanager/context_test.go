package taskmanager

import (
	"fmt"
	"sync"
	"testing"
)

func TestSharedContext_SetAndGet(t *testing.T) {
	sc := NewSharedContext()

	// Test setting and getting a value
	key := "test_key"
	value := "test_value"
	sc.Set(key, value)

	retrievedValue, ok := sc.Get(key)
	if !ok {
		t.Errorf("Expected to find key %s, but it was not found", key)
	}

	if retrievedValue != value {
		t.Errorf("Expected value %v, but got %v", value, retrievedValue)
	}
}

func TestSharedContext_GetNonExistent(t *testing.T) {
	sc := NewSharedContext()

	// Test getting a non-existent value
	_, ok := sc.Get("non_existent_key")
	if ok {
		t.Error("Expected key to not exist, but it was found")
	}
}

func TestSharedContext_ConcurrentAccess(t *testing.T) {
	sc := NewSharedContext()
	numGoroutines := 100
	numOperations := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // Writers and readers

	// Start writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				sc.Set(key, value)
			}
		}(i)
	}

	// Start readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				// Just attempt to read, don't validate since writers may not have written yet
				sc.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Verify some of the written data exists
	testKey := "key_0_0"
	expectedValue := "value_0_0"

	value, ok := sc.Get(testKey)
	if !ok {
		t.Errorf("Expected to find key %s after concurrent operations", testKey)
	}

	if value != expectedValue {
		t.Errorf("Expected value %s, but got %v", expectedValue, value)
	}
}
