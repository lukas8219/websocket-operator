package consistent_hashing

import (
	"fmt"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"testing"
)

type fixedPeers struct {
	peer_discovery.PeerDiscovery
	peers []peer_discovery.Peer
}

func (s fixedPeers) CurrentHosts() ([]peer_discovery.Peer, error) {
	return append([]peer_discovery.Peer(nil), s.peers...), nil
}

func FuzzJumpHashOrderIndependence(f *testing.F) {
	f.Add([]byte{1, 2, 3}, []byte("recipient"))
	f.Add([]byte{}, []byte{})
	f.Fuzz(func(t *testing.T, raw, key []byte) {
		if len(raw) > 64 || len(key) > 1024 {
			t.Skip()
		}
		peers := make([]peer_discovery.Peer, 0, len(raw))
		seen := map[byte]bool{}
		for _, v := range raw {
			if !seen[v] {
				seen[v] = true
				peers = append(peers, peer_discovery.NewPeer(fmt.Sprintf("10.0.0.%d", v), 3000))
			}
		}
		a, err := NewJumpHash(fixedPeers{peers: peers}).Lookup(key)
		if len(peers) == 0 {
			if err == nil {
				t.Fatal("empty membership succeeded")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range peers {
			if a == p {
				found = true
			}
		}
		if !found {
			t.Fatalf("selected absent peer: %v", a)
		}
		for i, j := 0, len(peers)-1; i < j; i, j = i+1, j-1 {
			peers[i], peers[j] = peers[j], peers[i]
		}
		b, err := NewJumpHash(fixedPeers{peers: peers}).Lookup(key)
		if err != nil || a != b {
			t.Fatalf("order changed route: %v / %v / %v", a, b, err)
		}
	})
}
