//go:build transportintegration

package transportlink

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"lukas8219/websocket-operator/internal/utils"
)

func connect(t *testing.T, addr, user string) (net.Conn, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var host string
	dialer := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{"X-Link-User": []string{user}}), OnHeader: func(key, value []byte) error {
		if bytes.EqualFold(key, []byte("x-ws-operator-upstream-host")) {
			host = string(value)
		}
		return nil
	}}
	conn, buffered, _, err := dialer.Dial(ctx, "ws://"+addr)
	if err != nil {
		t.Fatal(err)
	}
	conn = utils.WithBufferedReader(conn, buffered)
	t.Cleanup(func() { conn.Close() })
	return conn, host
}
func send(t *testing.T, conn net.Conn, op ws.OpCode, body []byte) {
	t.Helper()
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := wsutil.WriteClientMessage(conn, op, body); err != nil {
		t.Fatal(err)
	}
}
func receive(t *testing.T, conn net.Conn, opcode ws.OpCode, want []byte) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	body, op, err := wsutil.ReadServerData(conn)
	if err != nil || op != opcode || !bytes.Equal(body, want) {
		t.Fatalf("reply %q/%v: %v; want %q/%v", body, op, err, want, opcode)
	}
}
func selfRoundTrip(t *testing.T, conn net.Conn, user string) {
	t.Helper()
	body := []byte(fmt.Sprintf(`{"recipientId":%q,"body":"ready"}`, user))
	send(t, conn, ws.OpText, body)
	receive(t, conn, ws.OpText, body)
}
func TestSharedTransportProcesses(t *testing.T) {
	binaries := buildBinaries(t)
	for _, plugin := range []string{"http", "lavinmq"} {
		t.Run(plugin, func(t *testing.T) {
			if plugin == "lavinmq" && os.Getenv("LAVINMQ_AMQP_URL") == "" {
				t.Skip("set LAVINMQ_AMQP_URL for real-broker process tests")
			}
			c := startCluster(t, binaries, plugin)
			sender, senderHost := connect(t, c.lbs[0], "sender")
			selfRoundTrip(t, sender, "sender")
			var receiver net.Conn
			var user string
			for i := 0; i < 100; i++ {
				candidate := fmt.Sprintf("recipient-%d", i)
				conn, host := connect(t, c.lbs[1], candidate)
				selfRoundTrip(t, conn, candidate)
				if host != senderHost {
					receiver, user = conn, candidate
					break
				}
				conn.Close()
			}
			if receiver == nil {
				t.Fatal("could not select users on different sidecars")
			}
			eventually(t, func() bool { return c.applications[0].Load()+c.applications[1].Load() == 2 })
			for _, op := range []ws.OpCode{ws.OpText, ws.OpBinary} {
				body := []byte(fmt.Sprintf(`{"recipientId":%q,"body":"Olá\u0000","bytes":[0,255,128]}`, user))
				send(t, sender, op, body)
				receive(t, receiver, op, body)
				reply := []byte(`{"recipientId":"sender","body":"return to owner"}`)
				send(t, receiver, op, reply)
				receive(t, sender, op, reply)
			}
			// An owner crash removes only its sessions. The peer notices the vanished
			// AMQP inbox on its next heartbeat; direct links observe the socket close.
			c.owners[0].stop(t, true)
			eventually(t, func() bool { return c.applications[0].Load()+c.applications[1].Load() == 1 })
			selfRoundTrip(t, receiver, user)
			receiver.Close()
			eventually(t, func() bool { return c.applications[0].Load()+c.applications[1].Load() == 0 })
			// Reconnect gets a fresh session and no old reply or retained application.
			replacement, _ := connect(t, c.lbs[1], user)
			selfRoundTrip(t, replacement, user)
			replacement.Close()
			eventually(t, func() bool { return c.applications[0].Load()+c.applications[1].Load() == 0 })
		})
	}
}
