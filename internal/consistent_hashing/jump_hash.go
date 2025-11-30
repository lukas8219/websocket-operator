package consistent_hashing

import (
	"errors"
	"fmt"
	"hash"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"slices"
	"strings"

	"github.com/lithammer/go-jump-consistent-hash"
)

type JumpHashing struct {
	peerDiscoveryBacked peer_discovery.PeerDiscovery
	hasher              func() hash.Hash64
}

func NewJumpHash(peerDiscoveryBackend peer_discovery.PeerDiscovery) JumpHashing {
	return JumpHashing{
		peerDiscoveryBacked: peerDiscoveryBackend,
		hasher:              jump.NewCRC64,
	}
}

func (j JumpHashing) Lookup(Recipient []byte) (peer_discovery.Peer, error) {
	//TODO handle it
	currentHosts, err := j.peerDiscoveryBacked.CurrentHosts()
	slices.SortFunc(currentHosts, func(a, b peer_discovery.Peer) int {
		return strings.Compare(a.SocketAddres(), b.SocketAddres())
	})
	if err != nil {
		//We need to handle errrs
		return peer_discovery.Peer{}, err
	}
	if len(currentHosts) == 0 {
		return peer_discovery.Peer{}, errors.New("no hosts to lookup")
	}
	hasher := jump.NewCRC64()
	hasher.Write(Recipient)
	index := jump.Hash(
		hasher.Sum64(),
		int32(len(currentHosts)),
	)
	if index == -1 {
		return peer_discovery.Peer{}, fmt.Errorf("Didn't find any Peer to route")
	}
	return currentHosts[index], nil
}

func (j JumpHashing) Transaction(Add, Remove []peer_discovery.Peer) {
}
