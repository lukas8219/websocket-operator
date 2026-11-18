//go:build e2e

package system_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func TestConcurrentClients(t *testing.T) {
	waitForPeers(t, 3)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprintf("client-%d", i), func(t *testing.T) {
			t.Parallel()
			c := connect(t, fmt.Sprintf("e2e-concurrent-%d", i))
			deliver(t, c, c)
		})
	}
}

func scaleApplication(t *testing.T, count int) {
	t.Helper()
	cluster := endpoint(t, "WS_E2E_CLUSTER")
	if !strings.HasPrefix(cluster, "websocket-e2e-") {
		t.Fatal("refusing to scale outside the fixture's owned kind cluster")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "kubectl", "--context", "kind-"+cluster, "--namespace", endpoint(t, "WS_E2E_NAMESPACE"), "scale", "deployment/main-ws-server", fmt.Sprintf("--replicas=%d", count))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("scale application: %v\n%s", err, output)
	}
}

func TestScaleToZeroAndRecovery(t *testing.T) {
	waitForPeers(t, 3)
	existing := connect(t, "e2e-before-scaling")
	// Restore the fixture even if a shutdown assertion fails. Otherwise the
	// following routing tests fail against the intentionally empty deployment.
	restored := false
	t.Cleanup(func() {
		if !restored {
			scaleApplication(t, 3)
		}
	})
	scaleApplication(t, 0)
	waitForPeers(t, 0)
	request, err := http.NewRequest("GET", strings.Replace(endpoint(t, "WS_E2E_URL"), "ws://", "http://", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(endpoint(t, "WS_E2E_USER_HEADER"), "e2e-unavailable")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("no peers returned HTTP %d, want 503", response.StatusCode)
	}
	existing.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var body []byte
		var op ws.OpCode
		body, op, err = wsutil.ReadServerData(existing)
		if err != nil {
			break
		}
		// Removing peers can briefly rebalance onto a remaining sidecar. Its
		// application greeting can precede EOF in the TCP receive buffer.
		if op != ws.OpText || string(body) != "Welcome to the WebSocket server!" {
			t.Fatalf("unexpected message while awaiting close: %q/%v", body, op)
		}
	}
	if timed, ok := err.(net.Error); ok && timed.Timeout() {
		t.Fatalf("load balancer did not close the unroutable connection: %v", err)
	}
	scaleApplication(t, 3)
	restored = true
	waitForPeers(t, 3)
	recovered := connect(t, "e2e-after-scaling")
	deliver(t, recovered, recovered)
}
