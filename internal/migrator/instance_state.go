package migrator

import (
	"sync"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/maxkimambo/pd/internal/utils"
)

// InstanceStateManager provides thread-safe access to instance states
type InstanceStateManager struct {
	states map[string]InstanceState
	mutex  sync.RWMutex
}

// NewInstanceStateManager creates a new instance state manager
func NewInstanceStateManager() *InstanceStateManager {
	return &InstanceStateManager{
		states: make(map[string]InstanceState),
	}
}

// GetState returns the current state of an instance
// If the state is not cached, it determines the state from the instance's current status
func (m *InstanceStateManager) GetState(instance *computepb.Instance) InstanceState {
	key := getInstanceKey(instance)
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	state, exists := m.states[key]
	if !exists {
		// Use the existing getInstanceState function to determine current state
		return getInstanceState(instance)
	}

	return state
}

// SetState updates the state of an instance in the cache
func (m *InstanceStateManager) SetState(instance *computepb.Instance, state InstanceState) {
	key := getInstanceKey(instance)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.states[key] = state
}

// RemoveState removes an instance from the state manager
func (m *InstanceStateManager) RemoveState(instance *computepb.Instance) {
	key := getInstanceKey(instance)
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.states, key)
}

// GetAllStates returns a copy of all instance states
func (m *InstanceStateManager) GetAllStates() map[string]InstanceState {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	statesCopy := make(map[string]InstanceState, len(m.states))
	for k, v := range m.states {
		statesCopy[k] = v
	}

	return statesCopy
}

// HasState checks if an instance state is cached
func (m *InstanceStateManager) HasState(instance *computepb.Instance) bool {
	key := getInstanceKey(instance)
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.states[key]
	return exists
}

// Clear removes all cached states
func (m *InstanceStateManager) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.states = make(map[string]InstanceState)
}

// Count returns the number of cached instance states
func (m *InstanceStateManager) Count() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.states)
}

// getInstanceKey generates a unique key for an instance
// Format: project/zone/name
func getInstanceKey(instance *computepb.Instance) string {
	// Use the existing extractProjectFromSelfLink function from disk_migrator.go
	project := extractProjectFromSelfLink(instance.GetSelfLink())
	zone := utils.ExtractZoneName(instance.GetZone())
	return project + "/" + zone + "/" + instance.GetName()
}
