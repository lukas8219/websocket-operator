package consistent_hashing

import (
	"fmt"
	"lukas8219/websocket-operator/internal/peer_discovery"

	"github.com/lithammer/go-jump-consistent-hash"
)

type JumpHashing struct {
	peerDiscoveryBacked peer_discovery.PeerDiscovery
}

func NewJumpHash(peerDiscoveryBackend peer_discovery.PeerDiscovery) JumpHashing {
	return JumpHashing{
		peerDiscoveryBacked: peerDiscoveryBackend,
	}
}

func (j JumpHashing) Lookup(Recipient []byte) (peer_discovery.Peer, error) {
	//TODO handle it
	currentHosts, err := j.peerDiscoveryBacked.CurrentHosts()
	if err != nil {
		//We need to handle errrs
		return peer_discovery.Peer{}, err
	}
	var myUint64 uint64 = 1
	index := jump.Hash(
		myUint64,
		int32(len(currentHosts)),
	)
	if index == -1 {
		return peer_discovery.Peer{}, fmt.Errorf("Didn't find any Peer to route")
	}
	return currentHosts[index], nil
}

func (j JumpHashing) Transaction(Add, Remove []peer_discovery.Peer) {
}
