package main

import "sync"

// connectionRegistry protects membership while leaving network I/O outside the lock.
type connectionRegistry struct {
	mu          sync.RWMutex
	connections map[string]*ConnectionTracker
}

func newConnectionRegistry() *connectionRegistry {
	return &connectionRegistry{connections: make(map[string]*ConnectionTracker)}
}

func (r *connectionRegistry) get(user string) *ConnectionTracker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[user]
}

func (r *connectionRegistry) register(connection *ConnectionTracker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connections[connection.user] = connection
}

func (r *connectionRegistry) remove(connection *ConnectionTracker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Cleanup from an old session must not remove a replacement for the same user.
	if r.connections[connection.user] == connection {
		delete(r.connections, connection.user)
	}
}
