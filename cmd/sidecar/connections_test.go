package main

import (
	"strconv"
	"sync"
	"testing"
)

func TestConnectionRegistryPreservesReplacement(t *testing.T) {
	registry := newConnectionRegistry()
	old := &ConnectionTracker{user: "recipient"}
	replacement := &ConnectionTracker{user: "recipient"}
	registry.register(old)
	registry.register(replacement)
	registry.remove(old)
	if got := registry.get("recipient"); got != replacement {
		t.Fatalf("old session cleanup removed replacement: %p", got)
	}
	registry.remove(replacement)
	registry.remove(replacement)
	if registry.get("recipient") != nil || len(registry.connections) != 0 {
		t.Fatal("closed connection retained in registry")
	}
}

func TestConnectionRegistryConcurrentLifecycle(t *testing.T) {
	registry := newConnectionRegistry()
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 100; iteration++ {
				connection := &ConnectionTracker{user: strconv.Itoa(worker)}
				registry.register(connection)
				if registry.get(connection.user) != connection {
					t.Error("registered session is missing")
				}
				registry.get(strconv.Itoa((worker + 1) % 8))
				registry.remove(connection)
			}
		}(worker)
	}
	workers.Wait()
	if len(registry.connections) != 0 {
		t.Fatal("closed sessions retained in registry")
	}
}
