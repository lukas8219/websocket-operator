package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// Execute the real sidecar entry point in an isolated subprocess.
func TestBehaviorSidecarProcess(t *testing.T) {
	if os.Getenv("WS_BEHAVIOR_CHILD") != "1" {
		t.Skip("subprocess entry point")
	}
	flag.CommandLine = flag.NewFlagSet("sidecar", flag.ExitOnError)
	os.Args = []string{"sidecar", "-port", os.Getenv("WS_BEHAVIOR_PORT"), "-targetPort", os.Getenv("WS_BEHAVIOR_UPSTREAM")}
	main()
}

type sidecarFixture struct {
	url, logPath string
	client       *http.Client
}

func startBehaviorSidecar(t testing.TB) sidecarFixture {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("watch") == "true" {
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		io.WriteString(w, `{"kind":"EndpointsList","apiVersion":"v1","metadata":{"resourceVersion":"1"},"items":[]}`)
	}))
	t.Cleanup(api.Close)
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			data, opcode, err := wsutil.ReadClientData(conn)
			if err != nil {
				return
			}
			if wsutil.WriteServerMessage(conn, opcode, data) != nil {
				return
			}
		}
	}))
	t.Cleanup(echo.Close)
	_, upstream, err := net.SplitHostPort(echo.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	listener.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "kubeconfig")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster:\n    server: %s\ncontexts:\n- name: fixture\n  context:\n    cluster: fixture\ncurrent-context: fixture\n", api.URL)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "sidecar.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestBehaviorSidecarProcess$")
	child.Env = append(os.Environ(), "KUBERNETES_SERVICE_HOST=", "KUBERNETES_SERVICE_PORT=", "WS_BEHAVIOR_CHILD=1", "WS_BEHAVIOR_PORT="+port, "WS_BEHAVIOR_UPSTREAM="+upstream, "KUBECONFIG="+configPath)
	child.Stdout = logFile
	child.Stderr = logFile
	if err := child.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		child.Process.Kill()
		child.Wait()
		logFile.Close()
		logs, _ := os.ReadFile(logPath)
		if strings.Contains(string(logs), "WARNING: DATA RACE") || strings.Contains(string(logs), "fatal error:") {
			t.Errorf("sidecar subprocess reported a runtime failure:\n%s", logs)
		} else if t.Failed() {
			if len(logs) > 8000 {
				logs = logs[len(logs)-8000:]
			}
			t.Logf("sidecar subprocess log:\n%s", logs)
		}
	})
	address := net.JoinHostPort("127.0.0.1", port)
	ready := false
	for attempt := 0; attempt < 200; attempt++ {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		logs, _ := os.ReadFile(logPath)
		t.Fatalf("sidecar startup failed: %s", logs)
	}
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	return sidecarFixture{url: address, logPath: logPath, client: &http.Client{Transport: transport, Timeout: time.Second}}
}

func behaviorSidecarSession(fixture sidecarFixture, user string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dialer := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{"Ws-User-Id": []string{user}})}
	conn, _, _, err := dialer.Dial(ctx, "ws://"+fixture.url)
	if err != nil {
		return err
	}
	defer conn.Close()
	for attempt := 0; attempt < 100; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+fixture.url+"/message", bytes.NewReader([]byte{1, 'o', 'k'}))
		if err != nil {
			return err
		}
		req.Header.Set("ws-user-id", user)
		response, err := fixture.client.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode == http.StatusNotFound {
			time.Sleep(time.Millisecond)
			continue
		}
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("delivery HTTP %d", response.StatusCode)
		}
		conn.SetReadDeadline(time.Now().Add(time.Second))
		data, _, err := wsutil.ReadServerData(conn)
		if err != nil {
			return err
		}
		if string(data) != "ok" {
			return fmt.Errorf("echo: %q", data)
		}
		return nil
	}
	return fmt.Errorf("websocket %s not registered", user)
}
