package resolver

import (
	"fmt"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	peerDiscovery "lukas8219/websocket-operator/internal/peer_discovery"
	"sync/atomic"

	"k8s.io/utils/lru"
)

type ResolverVersion = int

type Resolver struct {
	consistentHashingAlgorithm consistent_hashing.ConsistentHashing[peerDiscovery.Peer]
	peerDiscovery.PeerDiscovery
	version atomic.Uint32
	cache   *lru.Cache
}

func New(
	peerDiscovery peerDiscovery.PeerDiscovery,
	consistentHashing consistent_hashing.ConsistentHashing[peerDiscovery.Peer],
) *Resolver {
	return &Resolver{
		consistentHashingAlgorithm: consistentHashing,
		PeerDiscovery:              peerDiscovery,
		cache:                      lru.New(1024),
		version:                    atomic.Uint32{},
	}
}

func (r *Resolver) Init() {
	for event := range r.NotificationChannel() {
		r.consistentHashingAlgorithm.Transaction(
			event.Added,
			event.Removed,
		)
		r.version.Add(1)
	}
}

func (r *Resolver) Lookup(Recipient []byte) (peerDiscovery.Peer, error) {
	_, error := r.CurrentHosts()
	if error != nil {
		return peerDiscovery.Peer{}, error
	}
	// TODO: investigate how to implemeny :relaxed memory access to prevent mem ordering here
	version := r.version.Load()
	cachedEntry, found := r.cache.Get(createCacheKey(version, Recipient))
	if found {
		return cachedEntry.(peerDiscovery.Peer), nil
	}
	peer, err := r.consistentHashingAlgorithm.Lookup(Recipient)
	if err != nil {
		return peerDiscovery.Peer{}, err
	}
	r.cache.Add(
		createCacheKey(version, Recipient),
		peer,
	)
	return peer, nil
}

func createCacheKey(version uint32, Recipient []byte) string {
	return fmt.Sprintf("%d:%s", version, Recipient)
}
