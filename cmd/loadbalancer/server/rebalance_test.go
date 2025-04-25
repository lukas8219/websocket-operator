package server

import (
	"bufio"
	"context"
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
	remoteAddr net.Addr
	isClosed   bool
}

func (m *NetConnectionMock) Read(b []byte) (int, error) {
	return 0, nil
}

func (m *NetConnectionMock) Write(b []byte) (int, error) {
	return 0, nil
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dialCalls = append(m.dialCalls, urlstr)
	mockConn := &NetConnectionMock{remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}}
	m.connections = append(m.connections, mockConn)
	return mockConn, nil, ws.Handshake{}, nil
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
		mockDownstreamConn := &NetConnectionMock{remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}}
		mockWSDialer := &MockWSDialer{}
		mockConn := NewMockConnection("user1", "old-host:3000", mockDownstreamConn, mockWSDialer)
		go mockConn.Handle()
		connections[mockConn.Tracker.User()] = mockConn
		mockConn.Tracker.UpstreamCancelChan() <- 1
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

	})

}
