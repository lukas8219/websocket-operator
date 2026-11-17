//go:build e2e

package system_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type client struct {
	net.Conn
	reader     io.Reader
	user, host string
}

func (c *client) Read(p []byte) (int, error) { return c.reader.Read(p) }

func endpoint(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required; run through scripts/test-system.sh", name)
	}
	return value
}

func waitForPeers(t *testing.T, count int) {
	t.Helper()
	httpClient := &http.Client{Timeout: 2 * time.Second}
	url := endpoint(t, "WS_E2E_HOSTS_URL")
	deadline := time.Now().Add(45 * time.Second)
	var observed string
	for time.Now().Before(deadline) {
		response, err := httpClient.Get(url)
		if err == nil {
			var peers []struct {
				Hostname string `json:"hostname"`
			}
			err = json.NewDecoder(response.Body).Decode(&peers)
			response.Body.Close()
			observed = fmt.Sprintf("status=%d peers=%v error=%v", response.StatusCode, peers, err)
			if err == nil && response.StatusCode == http.StatusOK && len(peers) == count {
				return
			}
		} else {
			observed = err.Error()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("discovery did not converge to %d peers: %s", count, observed)
}

func connect(t *testing.T, user string) *client {
	t.Helper()
	c := &client{user: user}
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP(http.Header{http.CanonicalHeaderKey(endpoint(t, "WS_E2E_USER_HEADER")): []string{user}}),
		OnHeader: func(key, value []byte) error {
			if strings.EqualFold(string(key), "x-ws-operator-upstream-host") {
				c.host = string(value)
			}
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, reader, _, err := dialer.Dial(ctx, endpoint(t, "WS_E2E_URL"))
	if err != nil {
		t.Fatalf("connect %s: %v", user, err)
	}
	c.Conn = conn
	c.reader = conn
	if reader != nil {
		c.reader = reader
	}
	t.Cleanup(func() { conn.Close() })
	if c.host == "" {
		t.Fatal("load balancer did not identify the selected peer")
	}
	// A successful self-delivery also proves the sidecar registered the recipient.
	deliver(t, c, c)
	return c
}

func deliver(t *testing.T, sender, recipient *client) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"recipientId": recipient.user, "message": fmt.Sprintf("e2e-%d", time.Now().UnixNano()),
	})
	if err != nil {
		t.Fatal(err)
	}
	sender.SetWriteDeadline(time.Now().Add(5 * time.Second))
	recipient.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := wsutil.WriteClientText(sender, payload); err != nil {
		t.Fatal(err)
	}
	expected := append([]byte("Server received: "), payload...)
	for {
		body, op, err := wsutil.ReadServerData(recipient)
		if err != nil {
			t.Fatalf("delivery %s (%s) -> %s (%s): %v", sender.user, sender.host, recipient.user, recipient.host, err)
		}
		if string(body) == "Welcome to the WebSocket server!" {
			continue
		}
		if op != ws.OpText || !bytes.Equal(body, expected) {
			t.Fatalf("recipient received opcode=%d body=%q; want %q", op, body, expected)
		}
		break
	}
}

func TestRoutedMessagesAcrossAllComponents(t *testing.T) {
	waitForPeers(t, 3)
	sender := connect(t, "e2e-sender")
	var local, remote *client
	for i := 0; i < 40 && (local == nil || remote == nil); i++ {
		candidate := connect(t, fmt.Sprintf("e2e-recipient-%d", i))
		if candidate.host == sender.host && local == nil {
			local = candidate
		} else if candidate.host != sender.host && remote == nil {
			remote = candidate
		} else {
			candidate.Close()
		}
	}
	if local == nil || remote == nil {
		t.Fatal("could not select both same-pod and cross-pod recipients")
	}
	t.Run("same pod", func(t *testing.T) { deliver(t, sender, local) })
	t.Run("cross pod HTTP forwarding", func(t *testing.T) { deliver(t, sender, remote) })
	t.Logf("sender=%s local=%s remote=%s", sender.host, local.host, remote.host)
}

func TestMissingRoutingIdentity(t *testing.T) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	response, err := httpClient.Get(strings.Replace(endpoint(t, "WS_E2E_URL"), "ws://", "http://", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing identity returned HTTP %d, want 400", response.StatusCode)
	}
}
