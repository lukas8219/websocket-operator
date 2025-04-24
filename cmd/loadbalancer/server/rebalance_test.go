package server

import (
	"log/slog"
	"lukas8219/websocket-operator/cmd/loadbalancer/connection"
	"net"
	"testing"
	"time"
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

type MockConnection struct {
	*connection.Connection
	upstreamHost       string
	user               string
	upstreamCancelChan chan struct{}
}

type MockProxier struct{}

func (m *MockProxier) ProxyDownstreamToUpstream() (net.Conn, error) {
	return nil, nil
}

func (m *MockProxier) ProxyUpstreamToDownstream() {

}

func NewMockConnection(user, upstreamHost string) *connection.Connection {
	conn := &net.TCPConn{}
	tracker := connection.NewTracker(user, upstreamHost, "downstream", conn)
	return &connection.Connection{
		Tracker: tracker,
		Proxier: &MockProxier{},
	}
}

func TestHandleRebalanceLoop(t *testing.T) {
	mockRouter := &MockRouter{
		rebalanceChan: make(chan [][2]string, 1),
	}
	connections := make(map[string]*connection.Connection)

	go handleRebalanceLoop(mockRouter, connections)

	t.Run("No connection tracker found", func(t *testing.T) {
		mockRouter.rebalanceChan <- [][2]string{{"non-existent", "new-host:3000"}}
	})

	t.Run("No need to rebalance - same host", func(t *testing.T) {
		mockConn := NewMockConnection("user1", "same-host:3000")
		connections["user1"] = mockConn

		mockRouter.rebalanceChan <- [][2]string{{"user1", "same-host:3000"}}
	})

	t.Run("Successfully received cancellation signal", func(t *testing.T) {
		mockConn := NewMockConnection("user2", "old-host:3000")
		connections["user2"] = mockConn

		go func() {
			mockConn.Tracker.UpstreamCancelChan() <- 1
		}()
		mockRouter.rebalanceChan <- [][2]string{{"user2", "new-host:3000"}}
		time.Sleep(100 * time.Millisecond)

		//Expect Switch Host + Handle to be called
		if mockConn.UpstreamHost() != "new-host:3000" {
			t.Errorf("Expected host to be updated to new-host:3000, got %s", mockConn.UpstreamHost())
		}
	})

	//TODO: change interface to use io.ReadWriter and inject Dialer so we can test new connections
}
