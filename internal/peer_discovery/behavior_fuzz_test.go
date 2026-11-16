package peer_discovery

import (
	"fmt"
	"io"
	"log/slog"
	"testing"
)

func FuzzKubernetesPeerSnapshots(f *testing.F) {
	f.Add([]byte{3, 1, 2, 3, 2, 2, 4, 0})
	f.Add([]byte{})
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.Cleanup(func() { slog.SetDefault(previousLogger) })
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 256 {
			t.Skip()
		}
		d := NewKubernetes(KubernetesPeerDiscoveryOptions{}).(*KubernetesPeerDiscovery)
		before := map[string]bool{}
		for pos := 0; pos < len(data); {
			count := int(data[pos] % 16)
			pos++
			next := map[string]bool{}
			hosts := []string{}
			old := []string{}
			for n := 0; n < count && pos < len(data); n++ {
				host := fmt.Sprintf("10.0.0.%d", data[pos])
				pos++
				hosts = append(hosts, host)
				next[host] = true
			}
			for h := range before {
				old = append(old, h)
			}
			d.updateHostsArray(hosts, old)
			peers, err := d.CurrentHosts()
			if err != nil {
				t.Fatal(err)
			}
			if len(peers) != len(next) {
				t.Fatalf("snapshot size %d, want %d", len(peers), len(next))
			}
			for _, p := range peers {
				if !next[p.Hostname()] || p.Port() != 3000 {
					t.Fatalf("unexpected peer %v", p)
				}
			}
			added, removed := map[string]bool{}, map[string]bool{}
			for h := range next {
				if !before[h] {
					added[h] = true
				}
			}
			for h := range before {
				if !next[h] {
					removed[h] = true
				}
			}
			select {
			case delta := <-d.NotificationChannel():
				if len(delta.Added) != len(added) || len(delta.Removed) != len(removed) {
					t.Fatalf("wrong delta %+v", delta)
				}
				for _, p := range delta.Added {
					if !added[p.Hostname()] {
						t.Fatalf("wrong added %v", p)
					}
				}
				for _, p := range delta.Removed {
					if !removed[p.Hostname()] {
						t.Fatalf("wrong removed %v", p)
					}
				}
			default:
				if len(added)+len(removed) != 0 {
					t.Fatal("missing update notification")
				}
			}
			before = next
		}
	})
}
