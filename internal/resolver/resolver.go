package resolver

import (
	peerDiscovery "lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/rendezvous"
)

type Resolver struct {
	hashingAlgorithm *rendezvous.Rendezvous
	peerDiscovery.PeerDiscovery
}

func New(
	peerDiscovery peerDiscovery.PeerDiscovery,
	hashingAlgorithm *rendezvous.Rendezvous,
) *Resolver {
	return nil
}

func (r *Resolver) Init() {
	for event := range r.NotificationChannel() {
		r.hashingAlgorithm.Transaction(
			event.Added,
			event.Removed,
		)
	}
}

func (r *Resolver) Lookup(Recipient []byte) (peerDiscovery.Peer, error) {
	_, error := r.CurrentHosts()
	if error != nil {
		return peerDiscovery.Peer{}, error
	}
	member := r.hashingAlgorithm.LocateKey(Recipient)
	return peerDiscovery.NewPeer(member.GetMember(), member.GetMember()), nil
}
