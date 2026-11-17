//go:build integration

package lavinmq

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func check(b *testing.B, err error) {
	b.Helper()
	if err != nil {
		b.Fatal(err)
	}
}

func channel(b *testing.B, address string) (*amqp.Connection, *amqp.Channel) {
	b.Helper()
	conn, err := amqp.DialConfig(address, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second)})
	check(b, err)
	b.Cleanup(func() { conn.Close() })
	ch, err := conn.Channel()
	check(b, err)
	return conn, ch
}

func httpSender(b *testing.B, packet []byte) func(string) error {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, packet) || r.Header.Get("ws-user-id") != "recipient" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Correlation-Id", r.Header.Get("X-Correlation-Id"))
		w.WriteHeader(http.StatusNoContent)
	}))
	b.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	return func(id string) error {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/message", bytes.NewReader(packet))
		if err != nil {
			return err
		}
		req.Header.Set("ws-user-id", "recipient")
		req.Header.Set("X-Correlation-Id", id)
		response, err := client.Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent || response.Header.Get("X-Correlation-Id") != id {
			return fmt.Errorf("HTTP acceptance mismatch: %s", response.Status)
		}
		return nil
	}
}

func amqpSender(b *testing.B, address string, packet []byte) func(string) error {
	server, receiver := channel(b, address)
	_, sender := channel(b, address)
	queue, err := receiver.QueueDeclare("", false, true, true, false, nil)
	check(b, err)
	reply, err := sender.QueueDeclare("", false, true, true, false, nil)
	check(b, err)
	check(b, receiver.Qos(1, 0, false))
	deliveries, err := receiver.Consume(queue.Name, "", false, true, false, false, nil)
	check(b, err)
	replies, err := sender.Consume(reply.Name, "", true, true, false, false, nil)
	check(b, err)
	failures := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for delivery := range deliveries {
			accepted := byte(1)
			if !bytes.Equal(delivery.Body, packet) || delivery.Headers["recipient"] != "recipient" {
				accepted = 0
			}
			// Reply only after receiver validation, not merely after broker acceptance.
			err := receiver.Publish("", delivery.ReplyTo, false, false, amqp.Publishing{CorrelationId: delivery.CorrelationId, Body: []byte{accepted}})
			if err == nil {
				err = delivery.Ack(false)
			}
			if err != nil {
				failures <- err
				return
			}
		}
	}()
	b.Cleanup(func() {
		server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			b.Error("consumer did not stop")
		}
	})
	return func(id string) error {
		if err := sender.Publish("", queue.Name, true, false, amqp.Publishing{
			ReplyTo: reply.Name, CorrelationId: id, Headers: amqp.Table{"recipient": "recipient"}, Body: packet,
		}); err != nil {
			return err
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case response, ok := <-replies:
			if !ok || response.CorrelationId != id || !bytes.Equal(response.Body, []byte{1}) {
				return fmt.Errorf("AMQP acceptance mismatch")
			}
			return nil
		case err := <-failures:
			return err
		case <-timer.C:
			return fmt.Errorf("AMQP receiver acceptance timed out")
		}
	}
}

func BenchmarkReceiverAcceptance(b *testing.B) {
	address := os.Getenv("LAVINMQ_AMQP_URL")
	if address == "" {
		b.Skip("set LAVINMQ_AMQP_URL to an isolated LavinMQ broker")
	}
	for _, size := range []int{128, 1024, 16384} {
		packet := append([]byte{2}, bytes.Repeat([]byte{0xa5}, size)...)
		for _, backend := range []string{"http", "amqp"} {
			b.Run(fmt.Sprintf("%s/bytes=%d", backend, size), func(b *testing.B) {
				var send func(string) error
				if backend == "http" {
					send = httpSender(b, packet)
				} else {
					send = amqpSender(b, address, packet)
				}
				check(b, send("warmup"))
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					check(b, send(strconv.Itoa(i)))
				}
			})
		}
	}
}
