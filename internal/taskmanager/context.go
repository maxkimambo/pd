package taskmanager

import "sync"

// SharedContext allows tasks to share data.
type SharedContext struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

// NewSharedContext creates a new SharedContext.
func NewSharedContext() *SharedContext {
	return &SharedContext{
		data: make(map[string]interface{}),
	}
}

// Set adds or updates a value in the context.
func (sc *SharedContext) Set(key string, value interface{}) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.data[key] = value
}

// Get retrieves a value from the context.
func (sc *SharedContext) Get(key string) (interface{}, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	val, ok := sc.data[key]
	return val, ok
}
