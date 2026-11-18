package builtin

import (
	"strings"
	"testing"

	"lukas8219/websocket-operator/internal/resolver"
)

type testResolver struct{ resolver.Resolver }

func TestSharedSelectionAndConfiguration(t *testing.T) {
	t.Setenv("WS_OPERATOR_NAMESPACE", "chat")
	t.Setenv("WS_OPERATOR_SERVICE", "peers")
	t.Setenv("WS_OPERATOR_POD_IP", "10.0.0.1")
	t.Setenv("WS_OPERATOR_USER_HEADER", "X-App-User")
	t.Setenv("WS_OPERATOR_TRANSPORT_PLUGIN", "")
	c, err := FromEnv("8080")
	if err != nil || c.Address != "10.0.0.1:8080" || c.Prefix != "ws-operator.link.chat.peers" || c.UserHeader != "X-App-User" {
		t.Fatalf("config %+v: %v", c, err)
	}
	endpoint, err := Open(c, Options{Resolver: testResolver{}})
	if err != nil {
		t.Fatal(err)
	}
	endpoint.Close()
	c.Name = "missing"
	if _, err := Open(c, Options{Resolver: testResolver{}}); err == nil {
		t.Fatal("unknown plugin silently fell back")
	}
	c.Name = "lavinmq"
	c.URL = "http://guest:private-password@localhost"
	if _, err := Open(c, Options{Resolver: testResolver{}}); err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("broker config error: %v", err)
	}
}
