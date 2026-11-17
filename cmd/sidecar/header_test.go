package main

import "testing"

func TestCustomHeaderHandshakeAndDelivery(t *testing.T) {
	fixture := startBehaviorSidecarWithHeader(t, "X-App-User")
	if err := behaviorSidecarSession(fixture, "custom-user"); err != nil {
		t.Fatal(err)
	}
}
