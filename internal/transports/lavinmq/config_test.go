package lavinmq

import (
	"strings"
	"testing"
)

func TestInvalidConfigurationDoesNotLeakCredentials(t *testing.T) {
	for _, url := range []string{"", "http://guest:private-password@localhost", "amqp://guest:private-password@localhost"} {
		_, err := New(Config{URL: url})
		if err == nil || strings.Contains(err.Error(), "private-password") {
			t.Fatalf("configuration error: %v", err)
		}
	}
}

func TestQueueIdentity(t *testing.T) {
	a := queueName("namespace.service", "10.0.0.1:3000")
	if a == queueName("namespace.service", "10.0.0.2:3000") || a == queueName("other.service", "10.0.0.1:3000") || len(a) > 255 {
		t.Fatal("queue identity must isolate endpoints and services")
	}
}
