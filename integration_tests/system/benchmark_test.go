//go:build e2e

package system_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func benchmarkPair(b *testing.B, path string) (*client, *client) {
	b.Helper()
	if path == "direct" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, reader, _, err := ws.Dial(ctx, endpoint(b, "WS_E2E_DIRECT_URL"))
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { conn.Close() })
		c := &client{Conn: conn, reader: conn, user: "benchmark-direct"}
		if reader != nil {
			c.reader = reader
		}
		deliver(b, c, c) // Consume the demo greeting and warm the connection.
		return c, c
	}
	waitForPeers(b, 3)
	sender := connect(b, "benchmark-sender")
	for i := 0; i < 40; i++ {
		receiver := connect(b, fmt.Sprintf("benchmark-recipient-%d", i))
		if (receiver.host == sender.host) == (path == "same-pod") {
			b.Logf("%s sender=%s receiver=%s", path, sender.host, receiver.host)
			return sender, receiver
		}
		receiver.Close()
	}
	b.Fatal("could not select the requested routing path")
	return nil, nil
}

func BenchmarkSystemRoundTrip(b *testing.B) {
	for _, path := range []string{"direct", "same-pod", "cross-pod"} {
		for _, size := range []int{128, 1024, 16384} {
			b.Run(fmt.Sprintf("%s/bytes=%d", path, size), func(b *testing.B) {
				sender, receiver := benchmarkPair(b, path)
				payload, err := json.Marshal(map[string]string{
					"recipientId": receiver.user, "message": strings.Repeat("x", size),
				})
				if err != nil {
					b.Fatal(err)
				}
				expected := append([]byte("Server received: "), payload...)
				latencies := make([]time.Duration, b.N)
				b.SetBytes(int64(len(payload)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					start := time.Now()
					sender.SetWriteDeadline(start.Add(5 * time.Second))
					receiver.SetReadDeadline(start.Add(5 * time.Second))
					if err := wsutil.WriteClientText(sender, payload); err != nil {
						b.Fatal(err)
					}
					body, op, err := wsutil.ReadServerData(receiver)
					if err != nil || op != ws.OpText || !bytes.Equal(body, expected) {
						b.Fatalf("invalid round trip: opcode=%d bytes=%d error=%v", op, len(body), err)
					}
					latencies[i] = time.Since(start)
				}
				b.StopTimer()
				sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
				for _, percentile := range []int{50, 95, 99} {
					b.ReportMetric(float64(latencies[(len(latencies)-1)*percentile/100].Nanoseconds())/1000, fmt.Sprintf("p%d-us", percentile))
				}
				b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "messages/s")
			})
		}
	}
}
