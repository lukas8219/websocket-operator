package server

import (
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"sync"
)

// Snapshot membership under the lock; never hold it during network operations.
type connectionRegistry struct {
	mu          sync.RWMutex
	connections map[string]*connection.Connection
}

func newConnectionRegistry() *connectionRegistry {
	return &connectionRegistry{connections: make(map[string]*connection.Connection)}
}
func (r *connectionRegistry) store(c *connection.Connection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[c.User()] = c
}
func (r *connectionRegistry) remove(c *connection.Connection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.connections[c.User()] == c {
		delete(r.connections, c.User())
	}
}
func (r *connectionRegistry) snapshot() []*connection.Connection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*connection.Connection, 0, len(r.connections))
	for _, c := range r.connections {
		result = append(result, c)
	}
	return result
}
