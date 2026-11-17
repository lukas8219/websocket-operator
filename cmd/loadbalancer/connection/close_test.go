package connection

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
)

func TestTerminalFramesCloseBothSockets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame ws.Frame
		code  ws.StatusCode
	}{
		{"normal close", ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "")), ws.StatusNormalClosure},
		{"fragmented control", ws.NewFrame(ws.OpPing, false, nil), ws.StatusProtocolError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			downstream, client := net.Pipe()
			upstream, echo := net.Pipe()
			for _, conn := range []net.Conn{downstream, client, upstream, echo} {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(2 * time.Second))
			}
			done := make(chan struct{})
			go func() {
				_proxyUpstreamToDownstream(context.Background(), "user", downstream, upstream)
				close(done)
			}()
			written := make(chan error, 1)
			go func() { written <- ws.WriteFrame(client, ws.MaskFrameInPlace(tc.frame)) }()
			frame, err := ws.ReadFrame(client)
			if err != nil {
				t.Fatal(err)
			}
			code, _ := ws.ParseCloseFrameData(frame.Payload)
			if frame.Header.OpCode != ws.OpClose || code != tc.code {
				t.Fatalf("close opcode=%v code=%v; want %v", frame.Header.OpCode, code, tc.code)
			}
			for _, conn := range []net.Conn{client, echo} {
				if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
					t.Fatalf("socket remained open: %v", err)
				}
			}
			if err := <-written; err != nil && tc.code == ws.StatusNormalClosure {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("proxy goroutine did not exit")
			}
		})
	}
}
