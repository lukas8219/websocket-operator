package main

import (
	"bytes"
	"context"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func FuzzSidecarMessageBody(f *testing.F) {
	f.Add([]byte{1, 'h', 'i'})
	fixture := startBehaviorSidecar(f)
	serial := 0
	f.Fuzz(func(t *testing.T, body []byte) {
		t.Cleanup(func() {
			if !t.Failed() {
				return
			}
			logs, _ := os.ReadFile(fixture.logPath)
			if len(logs) > 16000 {
				logs = logs[len(logs)-16000:]
			}
			t.Logf("sidecar subprocess log:\n%s", logs)
		})
		if len(body) > 2048 {
			t.Skip()
		}
		serial++
		user := strconv.Itoa(serial)
		dialer := ws.Dialer{Header: ws.HandshakeHeaderHTTP(http.Header{"Ws-User-Id": []string{user}})}
		conn, _, _, err := dialer.Dial(context.Background(), "ws://"+fixture.url)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		post := func(payload []byte) (int, error) {
			req, err := http.NewRequest(http.MethodPost, "http://"+fixture.url+"/message", bytes.NewReader(payload))
			if err != nil {
				return 0, err
			}
			req.Header.Set("ws-user-id", user)
			response, err := fixture.client.Do(req)
			if err != nil {
				return 0, err
			}
			defer response.Body.Close()
			io.Copy(io.Discard, response.Body)
			return response.StatusCode, nil
		}
		// A successful echo proves the websocket has been registered before the fuzz input.
		registered := false
		for attempt := 0; attempt < 100; attempt++ {
			status, err := post([]byte{1, 'o', 'k'})
			if err != nil {
				t.Fatal(err)
			}
			if status == 200 {
				registered = true
				break
			}
			if status != 404 {
				t.Fatalf("warmup HTTP %d", status)
			}
			time.Sleep(time.Millisecond)
		}
		if !registered {
			t.Fatal("websocket not registered")
		}
		conn.SetReadDeadline(time.Now().Add(time.Second))
		data, _, err := wsutil.ReadServerData(conn)
		if err != nil || string(data) != "ok" {
			t.Fatalf("warmup echo: %q %v", data, err)
		}
		status, err := post(body)
		if err != nil {
			logs, _ := os.ReadFile(fixture.logPath)
			if len(logs) > 4000 {
				logs = logs[len(logs)-4000:]
			}
			t.Fatalf("request body %x failed without an HTTP response: %v\n%s", body, err, logs)
		}
		if len(body) == 0 && status != http.StatusBadRequest {
			t.Fatalf("empty message body returned HTTP %d, want 400", status)
		}
	})
}
