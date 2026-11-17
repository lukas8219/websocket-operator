package config

import (
	"os"
	"testing"
)

func TestUserHeaderConfiguration(t *testing.T) {
	t.Setenv(UserHeaderEnv, "")
	if err := os.Unsetenv(UserHeaderEnv); err != nil {
		t.Fatal(err)
	}
	value, err := UserHeaderFromEnv()
	if err != nil || value != DefaultUserHeader {
		t.Fatalf("default: %q %v", value, err)
	}
	t.Setenv(UserHeaderEnv, "x-app-user")
	value, err = UserHeaderFromEnv()
	if err != nil || value != "X-App-User" {
		t.Fatalf("custom header: %q %v", value, err)
	}
	for _, invalid := range []string{"", "bad header", "x-user\r\ninjected", "Host", "Connection", "Upgrade", "Content-Length", "Sec-WebSocket-Key"} {
		t.Run(invalid, func(t *testing.T) {
			t.Setenv(UserHeaderEnv, invalid)
			if _, err := UserHeaderFromEnv(); err == nil {
				t.Fatalf("accepted invalid routing header %q", invalid)
			}
		})
	}
}
