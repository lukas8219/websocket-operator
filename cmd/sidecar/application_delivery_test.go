package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func TestConcurrentApplicationFramesAndControlReplies(t *testing.T) {
	local, app := net.Pipe()
	defer local.Close()
	defer app.Close()
	app.SetDeadline(time.Now().Add(5 * time.Second))
	connection := &ConnectionTracker{user: "recipient", upstreamConn: local}
	registry := newConnectionRegistry()
	registry.register(connection)
	received := make(chan error, 1)
	go func() {
		seen := make(map[string]bool)
		pongs := 0
		for i := 0; i < 33; i++ {
			frame, err := ws.ReadFrame(app)
			if err != nil {
				received <- err
				return
			}
			if !frame.Header.Masked || !frame.Header.Fin {
				received <- fmt.Errorf("invalid frame header: %+v", frame.Header)
				return
			}
			ws.Cipher(frame.Payload, frame.Header.Mask, 0)
			if frame.Header.OpCode == ws.OpPong {
				pongs++
				if string(frame.Payload) != "ping" {
					received <- fmt.Errorf("bad pong")
					return
				}
			} else {
				if frame.Header.OpCode != ws.OpBinary || len(frame.Payload) != 4096 || !bytes.Equal(frame.Payload, bytes.Repeat(frame.Payload[:1], 4096)) || seen[string(frame.Payload[:1])] {
					received <- fmt.Errorf("corrupt or duplicate frame")
					return
				}
				seen[string(frame.Payload[:1])] = true
			}
		}
		if pongs != 1 || len(seen) != 32 {
			received <- fmt.Errorf("frames=%d pongs=%d", len(seen), pongs)
			return
		}
		received <- nil
	}()
	readDone := make(chan error, 1)
	go func() {
		_, _, err := wsutil.ReadServerData(connection.applicationReader())
		readDone <- err
	}()
	var writers sync.WaitGroup
	for i := 0; i < 32; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			payload := bytes.Repeat([]byte{byte(i)}, 4096)
			var err error
			if i%2 == 0 {
				err = connection.deliverApplication(ws.OpBinary, payload)
			} else {
				err = localDelivery(registry)([]byte("recipient"), ws.OpBinary, payload)
			}
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	if err := wsutil.WriteServerMessage(app, ws.OpPing, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	writers.Wait()
	if err := <-received; err != nil {
		t.Fatal(err)
	}
	// End the read loop only after its automatic pong has been observed.
	if err := wsutil.WriteServerMessage(app, ws.OpText, []byte("done")); err != nil {
		t.Fatal(err)
	}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
}

// An immediate deadline simulates an application which never drains its socket,
// without making the test spend five seconds waiting for the production limit.
type expiredWriteConn struct{ net.Conn }

func (c expiredWriteConn) SetWriteDeadline(time.Time) error {
	return c.Conn.SetWriteDeadline(time.Now())
}

func TestFailedApplicationWriteClosesDamagedStream(t *testing.T) {
	local, app := net.Pipe()
	defer local.Close()
	defer app.Close()
	connection := &ConnectionTracker{upstreamConn: expiredWriteConn{local}}
	if err := connection.deliverApplication(ws.OpText, []byte("blocked")); err == nil {
		t.Fatal("blocked write succeeded")
	}
	app.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := app.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("failed stream remained open: %v", err)
	}
	if err := connection.deliverApplication(ws.OpText, []byte("later")); err == nil {
		t.Fatal("write reused damaged stream")
	}
}
