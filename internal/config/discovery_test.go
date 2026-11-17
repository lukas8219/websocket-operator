package config

import "testing"

func TestDiscoveryConfiguration(t *testing.T) {
	t.Setenv("WS_OPERATOR_NAMESPACE", "chat")
	t.Setenv("WS_OPERATOR_SERVICE", "chat-peers")
	got, err := DiscoveryFromEnv()
	if err != nil || got != (Discovery{Namespace: "chat", Service: "chat-peers"}) {
		t.Fatalf("discovery=%+v error=%v", got, err)
	}
	for _, bad := range []string{"", "UPPER", "has space", "a.b"} {
		t.Setenv("WS_OPERATOR_NAMESPACE", bad)
		if _, err := DiscoveryFromEnv(); err == nil {
			t.Fatalf("accepted namespace %q", bad)
		}
	}
}
