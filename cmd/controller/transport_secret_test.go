package main

import (
	"encoding/json"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func TestTransportSecretInjection(t *testing.T) {
	for _, secret := range []string{"", "broker-credentials"} {
		t.Run(secret, func(t *testing.T) {
			raw, err := createPatch(&appsv1.Deployment{}, "sidecar:test", "app-config", secret)
			if err != nil {
				t.Fatal(err)
			}
			var patch []struct{ Value corev1.Container }
			if err := json.Unmarshal(raw, &patch); err != nil {
				t.Fatal(err)
			}
			env := patch[0].Value.Env
			if env[0].ValueFrom.FieldRef.FieldPath != "status.podIP" {
				t.Fatal("missing shared endpoint identity")
			}
			if secret == "" {
				if len(env) != 1 {
					t.Fatal("default HTTP needs no secret")
				}
				return
			}
			if len(env) != 2 || env[1].Name != "WS_OPERATOR_LAVINMQ_URL" || env[1].Value != "" {
				t.Fatalf("bad secret env: %+v", env)
			}
			ref := env[1].ValueFrom.SecretKeyRef
			if ref.Name != secret || ref.Key != "WS_OPERATOR_LAVINMQ_URL" {
				t.Fatalf("bad secret reference: %+v", ref)
			}
		})
	}
}
