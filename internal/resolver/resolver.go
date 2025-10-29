package resolver

import (
	"fmt"
	"log/slog"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	peerDiscovery "lukas8219/websocket-operator/internal/peer_discovery"
	"sync/atomic"

	"k8s.io/utils/lru"
)

type ResolverVersion = uint32

type Resolver interface {
	peerDiscovery.PeerDiscovery
	Init()
	VersionUpgradeChannel() chan ResolverVersion
	Lookup([]byte) (peerDiscovery.Peer, error)
}

type ResolverImpl struct {
	consistentHashingAlgorithm consistent_hashing.ConsistentHashing[peerDiscovery.Peer]
	peerDiscovery.PeerDiscovery
	version               atomic.Uint32
	cache                 *lru.Cache
	versionUpgradeChannel chan ResolverVersion
}

func New(
	peerDiscovery peerDiscovery.PeerDiscovery,
	consistentHashing consistent_hashing.ConsistentHashing[peerDiscovery.Peer],
) Resolver {
	return &ResolverImpl{
		consistentHashingAlgorithm: consistentHashing,
		PeerDiscovery:              peerDiscovery,
		cache:                      lru.New(1024),
		version:                    atomic.Uint32{},
		versionUpgradeChannel:      make(chan ResolverVersion),
	}
}

func (r *ResolverImpl) VersionUpgradeChannel() chan ResolverVersion {
	return r.versionUpgradeChannel
}

func (r *ResolverImpl) Init() {
	go r.PeerDiscovery.Initialize()
	for event := range r.NotificationChannel() {
		r.consistentHashingAlgorithm.Transaction(
			event.Added,
			event.Removed,
		)
		new := r.version.Add(1)
		r.versionUpgradeChannel <- new
	}
}

func (r *ResolverImpl) Lookup(Recipient []byte) (peerDiscovery.Peer, error) {
	_, error := r.CurrentHosts()
	if error != nil {
		slog.Error("failed to lookup", "error", error)
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
