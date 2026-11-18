//go:build transportintegration

package transportlink

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type process struct {
	command *exec.Cmd
	ended   chan error
	once    sync.Once
}

func (p *process) stop(t *testing.T, crash bool) {
	t.Helper()
	p.once.Do(func() {
		if crash {
			p.command.Process.Kill()
		} else {
			p.command.Process.Signal(syscall.SIGTERM)
		}
		select {
		case err := <-p.ended:
			if !crash && err != nil {
				t.Errorf("process exit: %v", err)
			}
		case <-time.After(8 * time.Second):
			p.command.Process.Kill()
			<-p.ended
			t.Error("process did not stop")
		}
	})
}
func startProcess(t *testing.T, binary string, env []string, addr string, args ...string) *process {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), filepath.Base(binary)+".log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	p := &process{command: cmd, ended: make(chan error, 1)}
	go func() { p.ended <- cmd.Wait() }()
	t.Cleanup(func() {
		p.stop(t, false)
		log.Close()
		body, _ := os.ReadFile(logPath)
		if strings.Contains(string(body), "DATA RACE") || strings.Contains(string(body), "panic:") || strings.Contains(string(body), "fatal error:") {
			t.Errorf("runtime failure:\n%s", body)
		} else if t.Failed() {
			t.Logf("%s:\n%s", binary, body)
		}
	})
	eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
		}
		return err == nil
	})
	return p
}
func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return port
}
func buildBinaries(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	result := map[string]string{}
	for _, name := range []string{"sidecar", "loadbalancer"} {
		result[name] = filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-race", "-o", result[name], "./cmd/"+name)
		cmd.Dir = "../.."
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, output)
		}
	}
	return result
}

// Discovery uses a real client-go watch, backed by a small deterministic API.
// Network transport and the application/load-balancer/sidecar binaries are real.
func discoveryConfig(t *testing.T, service string) string {
	t.Helper()
	object := map[string]any{"kind": "Endpoints", "apiVersion": "v1", "metadata": map[string]any{"name": service, "namespace": "test", "resourceVersion": "1"}, "subsets": []any{map[string]any{"addresses": []any{map[string]any{"ip": "127.0.0.2"}, map[string]any{"ip": "127.0.0.3"}}, "ports": []any{map[string]any{"port": 3000}}}}}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		if r.URL.Query().Get("watch") == "true" {
			if r.URL.Query().Get("sendInitialEvents") == "true" {
				encoder.Encode(map[string]any{"type": "ADDED", "object": object})
				encoder.Encode(map[string]any{"type": "BOOKMARK", "object": map[string]any{"kind": "Endpoints", "apiVersion": "v1", "metadata": map[string]any{"resourceVersion": "1", "annotations": map[string]string{"k8s.io/initial-events-end": "true"}}}})
			}
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		encoder.Encode(map[string]any{"kind": "EndpointsList", "apiVersion": "v1", "metadata": map[string]string{"resourceVersion": "1"}, "items": []any{object}})
	}))
	t.Cleanup(api.Close)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	body := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: fixture\n  cluster:\n    server: %s\ncontexts:\n- name: fixture\n  context:\n    cluster: fixture\ncurrent-context: fixture\n", api.URL)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func echoApplication(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	active := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		active.Add(1)
		defer conn.Close()
		defer active.Add(-1)
		for {
			body, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				return
			}
			if wsutil.WriteServerMessage(conn, op, body) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())
	return port, active
}

type cluster struct {
	lbs          [2]string
	owners       [2]*process
	applications [2]*atomic.Int32
}

func startCluster(t *testing.T, binaries map[string]string, plugin string) *cluster {
	t.Helper()
	service := fmt.Sprintf("process-%d", time.Now().UnixNano())
	config := discoveryConfig(t, service)
	env := append(os.Environ(), "KUBERNETES_SERVICE_HOST=", "KUBERNETES_SERVICE_PORT=", "KUBECONFIG="+config, "WS_OPERATOR_NAMESPACE=test", "WS_OPERATOR_SERVICE="+service, "WS_OPERATOR_TRANSPORT_PLUGIN="+plugin, "WS_OPERATOR_USER_HEADER=X-Link-User", "WS_OPERATOR_LAVINMQ_URL="+os.Getenv("LAVINMQ_AMQP_URL"))
	c := &cluster{}
	for i, ip := range []string{"127.0.0.2", "127.0.0.3"} {
		port, active := echoApplication(t)
		c.applications[i] = active
		startProcess(t, binaries["sidecar"], append(append([]string{}, env...), "WS_OPERATOR_POD_IP="+ip), ip+":3000", "-listenHost", ip, "-targetPort", port)
	}
	for i := range c.lbs {
		port, admin := freePort(t), freePort(t)
		c.lbs[i] = "127.0.0.1:" + port
		c.owners[i] = startProcess(t, binaries["loadbalancer"], append(append([]string{}, env...), "WS_OPERATOR_POD_IP=127.0.0.1"), c.lbs[i], "-port", port, "-adminPort", admin)
		client := http.Client{Timeout: time.Second}
		eventually(t, func() bool {
			res, err := client.Get("http://127.0.0.1:" + admin)
			if err != nil {
				return false
			}
			defer res.Body.Close()
			var hosts []json.RawMessage
			return json.NewDecoder(res.Body).Decode(&hosts) == nil && len(hosts) == 2
		})
	}
	return c
}
