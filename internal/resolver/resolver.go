package resolver

import (
	"fmt"
	peerDiscovery "lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/rendezvous"
)

type Resolver struct {
	peerDiscovery    peerDiscovery.PeerDiscovery
	hashingAlgorithm *rendezvous.Rendezvous
}

func New(
	peerDiscovery peerDiscovery.PeerDiscovery,
	hashingAlgorithm *rendezvous.Rendezvous,
) *Resolver {
	return nil
}

func (r *Resolver) Init() {
	for event := range r.peerDiscovery.NotificationChannel() {
		// TODO handle NEW and DELETE Events
		fmt.Printf(string(len(event)))
	}
}

func (r *Resolver) Lookup(Recipient []byte) (peerDiscovery.Peer, error) {
	_, error := r.peerDiscovery.CurrentHosts()
	if error != nil {
		//we might need to return a pointer
		return peerDiscovery.Peer{}, error
	}
	member := r.hashingAlgorithm.LocateKey(Recipient)
	return peerDiscovery.NewPeer(member.GetMember(), member.GetMember()), nil
}
