package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestBehaviorSidecarConcurrentSessions(t *testing.T) {
	fixture := startBehaviorSidecar(t)
	var workers sync.WaitGroup
	for worker := 0; worker < 6; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for iteration := 0; iteration < 10; iteration++ {
				if err := behaviorSidecarSession(fixture, fmt.Sprintf("%d-%d", worker, iteration)); err != nil {
					t.Errorf("session: %v", err)
					return
				}
			}
		}(worker)
	}
	workers.Wait()
}
