package resolver

import (
	"fmt"
	"log/slog"
	"lukas8219/websocket-operator/internal/consistent_hashing"
	"lukas8219/websocket-operator/internal/peer_discovery"
	peerDiscovery "lukas8219/websocket-operator/internal/peer_discovery"
	"slices"
	"strings"
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

const (
	PEER_CACHE_KEY uint8 = 0
)

type ResolverImpl struct {
	consistentHashingAlgorithm consistent_hashing.ConsistentHashing[peerDiscovery.Peer]
	peerDiscovery.PeerDiscovery
	version               atomic.Uint32
	cache                 *lru.Cache
	peerHostsCache        *lru.Cache
	versionUpgradeChannel chan ResolverVersion
}

func New(
	peerDiscovery peerDiscovery.PeerDiscovery,
	consistentHashing consistent_hashing.ConsistentHashing[peerDiscovery.Peer],
) Resolver {
	return &ResolverImpl{
		consistentHashingAlgorithm: consistentHashing,
		PeerDiscovery:              peerDiscovery,
		cache:                      lru.New(4096),
		peerHostsCache:             lru.New(1),
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
		r.peerHostsCache.Clear()
		r.cache.Clear()
		slog.Debug("Cleansed cache", "version", r.version.Load())
		r.versionUpgradeChannel <- new
	}
}

func (r *ResolverImpl) CurrentHosts() ([]peerDiscovery.Peer, error) {
	_, found := r.peerHostsCache.Get(createPeerHostCacheKey(r.version.Load()))
	if found {
		slog.Debug("Cache hit", "version", r.version.Load())
		// return result.([]peerDiscovery.Peer), nil
	}
	hosts, err := r.PeerDiscovery.CurrentHosts()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(hosts, func(a, b peer_discovery.Peer) int {
		return strings.Compare(a.SocketAddres(), b.SocketAddres())
	})
	slog.Debug("add to cache", "version", r.version.Load())
	r.peerHostsCache.Add(createPeerHostCacheKey(r.version.Load()), hosts)
	return hosts, nil
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
	//Who reaaally needs to be versioned might be the consistent hashing output
	peer, err := r.consistentHashingAlgorithm.Lookup(Recipient)
	if err != nil {
		return peerDiscovery.Peer{}, err
	}
	r.cache.Add(
		createCacheKey(version, Recipient),
		peer,
	)
	slog.Debug("Returning from Lookup", "peer", peer.SocketAddres(), "version", r.version.Load())
	return peer, nil
}

func createPeerHostCacheKey(version uint32) string {
	return createCacheKey(version, []byte{PEER_CACHE_KEY})
}

func createCacheKey(version uint32, Suffix []byte) string {
	return fmt.Sprintf("%d:%s", version, Suffix)
}
