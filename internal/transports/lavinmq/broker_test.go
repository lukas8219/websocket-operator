package lavinmq

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gobwas/ws"
	amqp "github.com/rabbitmq/amqp091-go"
	"lukas8219/websocket-operator/internal/peer_discovery"
)

func brokerConfig(t *testing.T) Config {
	t.Helper()
	url := os.Getenv("LAVINMQ_AMQP_URL")
	if url == "" {
		t.Skip("set LAVINMQ_AMQP_URL for real-broker tests")
	}
	return Config{URL: url, Address: "127.0.0.1:3000", Prefix: fmt.Sprintf("test.%d", time.Now().UnixNano()), Timeout: time.Second,
		Lookup:  func([]byte) (peer_discovery.Peer, error) { return peer_discovery.NewPeer("127.0.0.1", 3001), nil },
		Deliver: func([]byte, ws.OpCode, []byte) error { return nil },
	}
}

func openBroker(t *testing.T, config Config) *Transport {
	t.Helper()
	transport, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { transport.Close() })
	return transport
}

func TestBrokerConcurrentDelivery(t *testing.T) {
	config := brokerConfig(t)
	sender := openBroker(t, config)
	config.Address = "127.0.0.1:3001"
	received := make(chan string, 64)
	config.Deliver = func(key []byte, op ws.OpCode, body []byte) error {
		if op != ws.OpBinary || !bytes.Equal(body, append([]byte{0, 255}, key...)) {
			return errors.New("corrupt envelope")
		}
		received <- string(key)
		return nil
	}
	receiver := openBroker(t, config)
	if duplicate, err := New(config); err == nil {
		duplicate.Close()
		t.Fatal("two receivers own the same queue")
	}
	var writers sync.WaitGroup
	for i := 0; i < 32; i++ {
		writers.Add(1)
		go func(i int) {
			defer writers.Done()
			key := []byte(fmt.Sprint(i))
			body := append([]byte{0, 255}, key...)
			original := bytes.Clone(body)
			if err := sender.Write(key, ws.OpBinary, body); err != nil {
				t.Error(err)
			}
			if !bytes.Equal(body, original) {
				t.Error("borrowed body mutated")
			}
			clear(key)
			clear(body) // Safe immediately after Write returns, even before delivery.
		}(i)
	}
	writers.Wait()
	seen := make(map[string]bool)
	for i := 0; i < 32; i++ {
		select {
		case key := <-received:
			if seen[key] {
				t.Fatalf("duplicate %q", key)
			}
			seen[key] = true
		case <-time.After(3 * time.Second):
			t.Fatal("delivery timeout")
		}
	}
	receiver.Close()
	sender.Close()
	if err := sender.Write([]byte("0"), ws.OpText, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("closed write: %v", err)
	}
}

func TestBrokerMissingQueueAndMalformedEnvelope(t *testing.T) {
	config := brokerConfig(t)
	sender := openBroker(t, config)
	if err := sender.Write([]byte("user"), ws.OpText, []byte("hello")); err == nil {
		t.Fatal("missing target accepted")
	}
	config.Address = "127.0.0.1:3001"
	delivered := make(chan string, 2)
	config.Deliver = func(_ []byte, _ ws.OpCode, body []byte) error {
		delivered <- string(body)
		return errors.New("application disconnected")
	}
	openBroker(t, config)
	conn, err := amqp.Dial(config.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	channel, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	// Invalid opcode and missing recipient must never reach the application.
	for _, headers := range []amqp.Table{{"recipient": "user", "opcode": int32(257)}, {"opcode": int32(ws.OpText)}} {
		if err := channel.Publish("", queueName(config.Prefix, config.Address), false, false, amqp.Publishing{Headers: headers, Body: []byte("bad")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sender.Write([]byte("user"), ws.OpText, []byte("valid")); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-delivered:
		if body != "valid" {
			t.Fatalf("malformed delivery: %q", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("consumer stopped after rejection")
	}
	select {
	case body := <-delivered:
		t.Fatalf("unexpected retry: %q", body)
	case <-time.After(100 * time.Millisecond):
	}
	// An abrupt socket loss must notify the host even when no Write is running.
	sender.socket.Close()
	select {
	case <-sender.Done():
	case <-time.After(time.Second):
		t.Fatal("loss not reported")
	}
}

func TestBrokerOverflow(t *testing.T) {
	config := brokerConfig(t)
	sender := openBroker(t, config)
	config.Address = "127.0.0.1:3001"
	blocked := make(chan struct{})
	defer close(blocked)
	config.Deliver = func([]byte, ws.OpCode, []byte) error { <-blocked; return nil }
	openBroker(t, config)
	rejected := false
	for i := 0; i < 260; i++ {
		if err := sender.Write([]byte("slow"), ws.OpBinary, nil); err != nil {
			rejected = true
			break
		}
	}
	if !rejected {
		t.Fatal("queue overflow accepted")
	}
}

func TestBrokerPublishTimeout(t *testing.T) {
	config := brokerConfig(t)
	uri, err := amqp.ParseURI(config.URL)
	if err != nil || uri.Scheme != "amqp" {
		t.Skip("TCP fault proxy requires amqp URL")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	stalled := atomic.Bool{}
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		client, err := listener.Accept()
		if err != nil {
			return
		}
		defer client.Close()
		broker, err := net.Dial("tcp", net.JoinHostPort(uri.Host, fmt.Sprint(uri.Port)))
		if err != nil {
			return
		}
		defer broker.Close()
		go func() { io.Copy(broker, client); broker.Close() }()
		buffer := make([]byte, 32768)
		for {
			n, err := broker.Read(buffer)
			if err != nil {
				return
			}
			if stalled.Load() {
				<-finished
				return
			}
			if _, err = client.Write(buffer[:n]); err != nil {
				return
			}
		}
	}()
	proxyURL, _ := url.Parse(config.URL)
	proxyURL.Host = listener.Addr().String()
	config.URL = proxyURL.String()
	config.Timeout = 300 * time.Millisecond
	sender := openBroker(t, config)
	stalled.Store(true)
	started := time.Now()
	if err := sender.Write([]byte("user"), ws.OpText, []byte("uncertain")); err == nil {
		t.Fatal("unconfirmed publish succeeded")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("publish timeout did not bound socket I/O")
	}
	select {
	case <-sender.Done():
	case <-time.After(time.Second):
		t.Fatal("ambiguous publish did not stop transport")
	}
}
