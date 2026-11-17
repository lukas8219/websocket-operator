package peer_discovery

import "testing"

func TestPeerSocketAddressFamilies(t *testing.T) {
	for _, tc := range []struct {
		host string
		port uint16
		want string
	}{
		{"127.0.0.1", 3000, "127.0.0.1:3000"},
		{"peer.example", 443, "peer.example:443"},
		{"::1", 3000, "[::1]:3000"},
		{"fe80::1%eth0", 65535, "[fe80::1%eth0]:65535"},
		{"2001:db8::1", 0, "[2001:db8::1]:0"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			if got := NewPeer(tc.host, tc.port).SocketAddres(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
