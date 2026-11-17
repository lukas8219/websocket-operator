package server

import (
	"fmt"
	"sync"
	"testing"

	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
)

type registryCandidate interface {
	store(*connection.Connection)
	remove(*connection.Connection)
	visit(func(*connection.Connection))
}

type snapshotCandidate struct{ *connectionRegistry }

func (r snapshotCandidate) visit(fn func(*connection.Connection)) {
	for _, c := range r.snapshot() {
		fn(c)
	}
}

// Only the benchmark uses sync.Map; production continues to use connectionRegistry.
type rangeCandidate struct{ connections sync.Map }

func (r *rangeCandidate) store(c *connection.Connection) { r.connections.Store(c.User(), c) }
func (r *rangeCandidate) remove(c *connection.Connection) {
	r.connections.CompareAndDelete(c.User(), c)
}
func (r *rangeCandidate) visit(fn func(*connection.Connection)) {
	r.connections.Range(func(_, value any) bool { fn(value.(*connection.Connection)); return true })
}

// These serial workloads isolate allocation costs without network I/O or contention.
func BenchmarkRegistryComparison(b *testing.B) {
	for _, size := range []int{1000, 10000, 100000} {
		connections := make([]*connection.Connection, size)
		for i := range connections {
			connections[i] = &connection.Connection{Tracker: connection.NewTracker(fmt.Sprintf("user-%d", i), "host", "client", nil)}
		}
		for _, candidate := range []struct {
			name   string
			create func() registryCandidate
		}{
			{"snapshot", func() registryCandidate { return snapshotCandidate{newConnectionRegistry()} }},
			{"sync-map", func() registryCandidate { return &rangeCandidate{} }},
		} {
			b.Run(fmt.Sprintf("%s/connections=%d", candidate.name, size), func(b *testing.B) {
				registry := candidate.create()
				for _, c := range connections {
					registry.store(c)
				}
				b.Run("walk", func(b *testing.B) {
					seen := 0
					visit := func(c *connection.Connection) {
						if c != nil {
							seen++
						}
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						seen = 0
						registry.visit(visit)
						if seen != size {
							b.Fatalf("visited %d connections, want %d", seen, size)
						}
					}
				})
				b.Run("remove-and-store", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						c := connections[i%size]
						registry.remove(c)
						registry.store(c)
					}
				})
				b.Run("populate", func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						r := candidate.create()
						for _, c := range connections {
							r.store(c)
						}
					}
				})
			})
		}
	}
}
