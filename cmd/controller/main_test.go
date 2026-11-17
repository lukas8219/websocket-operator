package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAdmissionInjectionAndDryRun(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		enabled, existing, dryRun bool
	}{
		{"injection", true, false, false}, {"unlabeled", false, false, false},
		{"already injected", true, true, false}, {"dry run", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewClientset()
			deployment := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Labels: map[string]string{}}, Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "example/app:v1"}}}},
			}}
			if tc.enabled {
				deployment.Labels[labelKey] = "true"
			}
			if tc.existing {
				deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, corev1.Container{Name: "websocket-proxy"})
			}
			raw, err := json.Marshal(deployment)
			if err != nil {
				t.Fatal(err)
			}
			review := admissionv1.AdmissionReview{TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"}, Request: &admissionv1.AdmissionRequest{
				UID: "request-1", Kind: metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Object: runtime.RawExtension{Raw: raw}, DryRun: &tc.dryRun,
			}}
			body, err := json.Marshal(review)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", "/mutate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			NewSidecarInjector(client, "example/sidecar:revision").sidecarInjectionHandler(response, req)
			if response.Code != 200 {
				t.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
			}
			var result admissionv1.AdmissionReview
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Response == nil || !result.Response.Allowed || result.Response.UID != review.Request.UID {
				t.Fatalf("response: %+v", result.Response)
			}
			inject := tc.enabled && !tc.existing
			if inject {
				var patch []struct {
					Op, Path string
					Value    corev1.Container
				}
				if err := json.Unmarshal(result.Response.Patch, &patch); err != nil {
					t.Fatal(err)
				}
				if len(patch) != 1 || patch[0].Path != "/spec/template/spec/containers/-" || patch[0].Value.Image != "example/sidecar:revision" {
					t.Fatalf("patch: %+v", patch)
				}
			} else if len(result.Response.Patch) != 0 {
				t.Fatal("unexpected injection patch")
			}
			services, err := client.CoreV1().Services("default").List(context.Background(), metav1.ListOptions{})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if inject && !tc.dryRun {
				want = 1
			}
			if len(services.Items) != want {
				t.Fatalf("services=%d, want %d", len(services.Items), want)
			}
		})
	}
}
