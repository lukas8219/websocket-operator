package server

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
)

type MockRouter struct {
	rebalanceChan chan [][2]string
	*slog.Logger
}

func (m *MockRouter) RebalanceRequests() <-chan [][2]string {
	return m.rebalanceChan
}

func (m *MockRouter) Route(string) string { return "" }
func (m *MockRouter) Add([]string)        {}
func (m *MockRouter) GetAllUpstreamHosts() []string {
	return []string{}
}
func (m *MockRouter) InitializeHosts() error { return nil }

type NetConnectionMock struct {
	net.Conn
	remoteAddr    net.Addr
	isClosed      bool
	name          string
	receivedBytes []byte
	isServer      bool
}

func (m *NetConnectionMock) Read(b []byte) (int, error) {
	message := []byte(m.name)

	frame := ws.NewBinaryFrame(message)

	// If masked, apply masking
	if !m.isServer {
		frame = ws.MaskFrame(frame)
	}
	compiledFrame, err := ws.CompileFrame(frame)
	if err != nil {
		return 0, err
	}
	return copy(b, compiledFrame), nil
}

func (m *NetConnectionMock) Write(b []byte) (int, error) {
	m.receivedBytes = append(m.receivedBytes, b...)
	return len(b), nil
}

func (m *NetConnectionMock) RemoteAddr() net.Addr {
	return m.remoteAddr
}

func (m *NetConnectionMock) Close() error {
	m.isClosed = true
	return nil
}

type MockWSDialer struct {
	dialCalls   []string
	connections []*NetConnectionMock
	mu          sync.RWMutex
}

func (m *MockWSDialer) Dial(ctx context.Context, urlstr string) (net.Conn, *bufio.Reader, ws.Handshake, error) {
	log.Println("Dialing", urlstr)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dialCalls = append(m.dialCalls, urlstr)
	mockConn := &NetConnectionMock{remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}, name: urlstr, isServer: true}
	m.connections = append(m.connections, mockConn)
	reader := bufio.NewReader(mockConn)
	return mockConn, reader, ws.Handshake{}, nil
}

func NewMockConnection(user, upstreamHost string, downstreamConn net.Conn, wsDialer *MockWSDialer) *connection.Connection {
	tracker := connection.NewTracker(user, upstreamHost, "downstream", downstreamConn)
	return &connection.Connection{
		Tracker: tracker,
		Proxier: connection.NewWSProxier(tracker, wsDialer),
	}
}

func TestHandleRebalanceLoop(t *testing.T) {
	mockRouter := &MockRouter{
		rebalanceChan: make(chan [][2]string, 1),
	}
	connections := make(map[string]*connection.Connection)

	go handleRebalanceLoop(mockRouter, connections)

	t.Run("Sucessfully rebalanced", func(t *testing.T) {
		mockDownstreamConn := &NetConnectionMock{remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}, name: "old-host:3000", isServer: false}
		mockWSDialer := &MockWSDialer{}
		mockConn := NewMockConnection("user1", "old-host:3000", mockDownstreamConn, mockWSDialer)
		go mockConn.Handle()
		connections[mockConn.Tracker.User()] = mockConn

		mockConn.Tracker.UpstreamCancelChan() <- 1
		time.Sleep(100 * time.Millisecond)
		mockRouter.rebalanceChan <- [][2]string{{mockConn.Tracker.User(), "new-host:3000"}}
		time.Sleep(100 * time.Millisecond)

		if mockConn.UpstreamHost() != "new-host:3000" {
			t.Errorf("Expected host to be updated to new-host:3000, got %s", mockConn.UpstreamHost())
		}
		if len(mockWSDialer.dialCalls) != 2 {
			t.Errorf("Expected dial to be called twice, got %d", len(mockWSDialer.dialCalls))
			return
		}
		if mockWSDialer.dialCalls[1] != "ws://new-host:3000" {
			t.Errorf("Expected dial to be called with ws://new-host:3000, got %s", mockWSDialer.dialCalls[1])
		}

		reader := bytes.NewReader(mockDownstreamConn.receivedBytes)
		frame, err := ws.ReadFrame(reader)
		if err != nil {
			t.Errorf("Expected no error, got %s", err)
		}
		log.Println(string(frame.Payload))
	})

}
