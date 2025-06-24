package migrator

import (
	"sync"
	"testing"

	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestInstanceStateManager_NewInstanceStateManager(t *testing.T) {
	manager := NewInstanceStateManager()

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.states)
	assert.Equal(t, 0, manager.Count())
}

func TestInstanceStateManager_SetAndGetState(t *testing.T) {
	manager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	// Initially, should return state based on instance status
	initialState := manager.GetState(instance)
	assert.Equal(t, InstanceStateRunning, initialState)
	assert.False(t, manager.HasState(instance))

	// Set a different state
	manager.SetState(instance, InstanceStateStopped)

	// Should now return the cached state
	cachedState := manager.GetState(instance)
	assert.Equal(t, InstanceStateStopped, cachedState)
	assert.True(t, manager.HasState(instance))
}

func TestInstanceStateManager_RemoveState(t *testing.T) {
	manager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	// Set state
	manager.SetState(instance, InstanceStateStopped)
	assert.True(t, manager.HasState(instance))
	assert.Equal(t, 1, manager.Count())

	// Remove state
	manager.RemoveState(instance)
	assert.False(t, manager.HasState(instance))
	assert.Equal(t, 0, manager.Count())

	// Should fall back to instance status
	state := manager.GetState(instance)
	assert.Equal(t, InstanceStateRunning, state)
}

func TestInstanceStateManager_GetAllStates(t *testing.T) {
	manager := NewInstanceStateManager()

	instance1 := &computepb.Instance{
		Name:     proto.String("instance-1"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/instance-1"),
		Status:   proto.String("RUNNING"),
	}

	instance2 := &computepb.Instance{
		Name:     proto.String("instance-2"),
		Zone:     proto.String("projects/test-project/zones/us-west1-b"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-b/instances/instance-2"),
		Status:   proto.String("TERMINATED"),
	}

	// Set states
	manager.SetState(instance1, InstanceStateRunning)
	manager.SetState(instance2, InstanceStateStopped)

	allStates := manager.GetAllStates()
	assert.Len(t, allStates, 2)

	// Verify it's a copy, not the original map (compare addresses)
	assert.NotSame(t, &manager.states, &allStates)

	// Modify the returned map and ensure original is unchanged
	allStates["test-key"] = InstanceStateUnknown
	assert.Equal(t, 2, manager.Count())
}

func TestInstanceStateManager_Clear(t *testing.T) {
	manager := NewInstanceStateManager()

	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	// Set state
	manager.SetState(instance, InstanceStateStopped)
	assert.Equal(t, 1, manager.Count())

	// Clear all states
	manager.Clear()
	assert.Equal(t, 0, manager.Count())
	assert.False(t, manager.HasState(instance))
}

func TestInstanceStateManager_ConcurrentAccess(t *testing.T) {
	manager := NewInstanceStateManager()

	// Create multiple instances
	instances := make([]*computepb.Instance, 10)
	for i := 0; i < 10; i++ {
		instances[i] = &computepb.Instance{
			Name:     proto.String("instance-" + string(rune(i+'0'))),
			Zone:     proto.String("projects/test-project/zones/us-west1-a"),
			SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/instance-" + string(rune(i+'0'))),
			Status:   proto.String("RUNNING"),
		}
	}

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			manager.SetState(instances[idx], InstanceStateRunning)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = manager.GetState(instances[idx])
		}(i)
	}

	// Concurrent state checks
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = manager.HasState(instances[idx])
		}(i)
	}

	wg.Wait()

	// All instances should be in the manager
	assert.Equal(t, 10, manager.Count())

	// Test concurrent removal
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			manager.RemoveState(instances[idx])
		}(i)
	}

	wg.Wait()

	// Should have 5 instances left
	assert.Equal(t, 5, manager.Count())
}

func TestGetInstanceKey(t *testing.T) {
	tests := []struct {
		name     string
		instance *computepb.Instance
		expected string
	}{
		{
			name: "Valid instance with self link",
			instance: &computepb.Instance{
				Name:     proto.String("my-instance"),
				Zone:     proto.String("projects/my-project/zones/us-west1-a"),
				SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/my-project/zones/us-west1-a/instances/my-instance"),
			},
			expected: "my-project/us-west1-a/my-instance",
		},
		{
			name: "Instance without self link",
			instance: &computepb.Instance{
				Name: proto.String("my-instance"),
				Zone: proto.String("projects/my-project/zones/us-west1-a"),
			},
			expected: "/us-west1-a/my-instance",
		},
		{
			name: "Instance with minimal info",
			instance: &computepb.Instance{
				Name: proto.String("test"),
				Zone: proto.String("us-west1-a"),
			},
			expected: "/us-west1-a/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getInstanceKey(tt.instance)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInstanceStateManager_DifferentInstances(t *testing.T) {
	manager := NewInstanceStateManager()

	// Same name, different zones
	instance1 := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	instance2 := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-b"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-b/instances/test-instance"),
		Status:   proto.String("TERMINATED"),
	}

	// Same name, different projects
	instance3 := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/other-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/other-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("SUSPENDED"),
	}

	// Set different states
	manager.SetState(instance1, InstanceStateRunning)
	manager.SetState(instance2, InstanceStateStopped)
	manager.SetState(instance3, InstanceStateSuspended)

	// Should be treated as different instances
	assert.Equal(t, 3, manager.Count())
	assert.Equal(t, InstanceStateRunning, manager.GetState(instance1))
	assert.Equal(t, InstanceStateStopped, manager.GetState(instance2))
	assert.Equal(t, InstanceStateSuspended, manager.GetState(instance3))
}

func TestInstanceStateManager_StateOverride(t *testing.T) {
	manager := NewInstanceStateManager()

	// Instance with RUNNING status
	instance := &computepb.Instance{
		Name:     proto.String("test-instance"),
		Zone:     proto.String("projects/test-project/zones/us-west1-a"),
		SelfLink: proto.String("https://www.googleapis.com/compute/v1/projects/test-project/zones/us-west1-a/instances/test-instance"),
		Status:   proto.String("RUNNING"),
	}

	// Initially should return RUNNING based on status
	assert.Equal(t, InstanceStateRunning, manager.GetState(instance))

	// Override with custom state
	manager.SetState(instance, InstanceStateStopped)

	// Should return the overridden state
	assert.Equal(t, InstanceStateStopped, manager.GetState(instance))

	// Remove override
	manager.RemoveState(instance)

	// Should fall back to instance status
	assert.Equal(t, InstanceStateRunning, manager.GetState(instance))
}
