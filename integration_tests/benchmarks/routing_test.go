package benchmarks

import (
	"fmt"
	"testing"

	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/resolver"
)

func BenchmarkRouting(b *testing.B) {
	for _, count := range []int{8, 64, 256} {
		discovery := peer_discovery.NewInMemoryPeerDiscovery()
		go func() {
			for range discovery.NotificationChannel() {
			}
		}()
		for i := 0; i < count; i++ {
			discovery.AddPeer(peer_discovery.NewPeer(fmt.Sprintf("10.0.%d.%d", i/256, i%256), 3000))
		}
		close(discovery.NotificationChannel())
		hash := consistent_hashing.NewJumpHash(discovery)
		routes := resolver.New(discovery, hash)
		keys := make([][]byte, 1024)
		for i := range keys {
			keys[i] = []byte(fmt.Sprintf("benchmark-user-%04d", i))
		}
		for _, target := range []struct {
			name   string
			lookup func([]byte) (peer_discovery.Peer, error)
		}{{"hash", hash.Lookup}, {"resolver", routes.Lookup}} {
			b.Run(fmt.Sprintf("%s/peers=%d", target.name, count), func(b *testing.B) {
				// Warm all keys: the resolver's current cache behavior is measured as-is.
				for _, key := range keys {
					if _, err := target.lookup(key); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if peer, err := target.lookup(keys[i%len(keys)]); err != nil || peer.Port() != 3000 {
						b.Fatalf("invalid route: %v %v", peer, err)
					}
				}
			})
		}
	}
}
