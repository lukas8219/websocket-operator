package peer_discovery

import (
	"net"
	"strconv"
	"testing"
)

func FuzzPeerSocketAddress(f *testing.F) {
	f.Add([]byte{127, 0, 0, 1}, uint16(3000))
	f.Fuzz(func(t *testing.T, raw []byte, port uint16) {
		if len(raw) != net.IPv4len && len(raw) != net.IPv6len {
			t.Skip()
		}
		ip := net.IP(raw).String()
		peer := NewPeer(ip, port)
		host, portString, err := net.SplitHostPort(peer.SocketAddres())
		if err != nil {
			t.Fatalf("invalid address %q for IP %q: %v", peer.SocketAddres(), ip, err)
		}
		if !net.ParseIP(host).Equal(net.ParseIP(ip)) || portString != strconv.Itoa(int(port)) {
			t.Fatalf("address changed peer: %s", peer.SocketAddres())
		}
	})
}
