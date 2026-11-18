package link

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func TestWebSocketSessionConcurrentFrames(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	b.SetDeadline(time.Now().Add(3 * time.Second))
	sender, receiver := WebSocket(a, ws.StateClientSide), WebSocket(b, ws.StateServerSide)
	var writers sync.WaitGroup
	for i := 0; i < 16; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			body := bytes.Repeat([]byte{byte(i)}, 4096)
			if err := sender.Write(ws.OpBinary, body); err != nil {
				t.Error(err)
			}
			if !bytes.Equal(body, bytes.Repeat([]byte{byte(i)}, 4096)) {
				t.Error("borrowed payload changed")
			}
		}(i)
	}
	seen := map[byte]bool{}
	for i := 0; i < 16; i++ {
		body, op, err := receiver.Read()
		if err != nil {
			t.Fatal(err)
		}
		if op != ws.OpBinary || len(body) != 4096 || !bytes.Equal(body, bytes.Repeat(body[:1], 4096)) || seen[body[0]] {
			t.Fatal("corrupt/duplicate frame")
		}
		seen[body[0]] = true
	}
	writers.Wait()
}

func TestWebSocketSessionControlAndClose(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	b.SetDeadline(time.Now().Add(3 * time.Second))
	session := WebSocket(a, ws.StateServerSide)
	result := make(chan error, 1)
	go func() { _, _, err := session.Read(); result <- err }()
	if err := wsutil.WriteClientMessage(b, ws.OpPing, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	frame, err := ws.ReadFrame(b)
	if err != nil || frame.Header.OpCode != ws.OpPong || string(frame.Payload) != "ping" {
		t.Fatalf("pong: %+v %v", frame, err)
	}
	session.Close()
	session.Close()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("closed read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not unblock reader")
	}
}
