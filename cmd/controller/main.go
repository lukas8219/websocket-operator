package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	port       int
	certDir    string
	tlsEnabled bool
)

const (
	labelKey = "ws.operator/enabled"
)

func init() {
	flag.IntVar(&port, "port", 8080, "The port for the webhook server to listen on.")
	flag.StringVar(&certDir, "cert-dir", "/etc/webhook/certs", "The directory containing the TLS certificates.")
	flag.BoolVar(&tlsEnabled, "tls-enabled", true, "Whether to enable TLS. Set to false for development only.")
	flag.Parse()
}

// SidecarInjector handles admission webhook requests
type SidecarInjector struct {
	decoder runtime.Decoder
}

// NewSidecarInjector creates a new SidecarInjector
func NewSidecarInjector() *SidecarInjector {
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	return &SidecarInjector{
		decoder: codecs.UniversalDeserializer(),
	}
}

// sidecarInjectionHandler handles the http request for the admission webhook
func (si *SidecarInjector) sidecarInjectionHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling webhook request")

	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		log.Println("Empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// Verify content type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("Content-Type=%s, expecting application/json", contentType)
		http.Error(w, "invalid Content-Type, want application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Parse AdmissionReview request
	var admissionReview admissionv1.AdmissionReview
	if _, _, err := si.decoder.Decode(body, nil, &admissionReview); err != nil {
		log.Printf("Could not decode body: %v", err)
		http.Error(w, "could not decode body", http.StatusBadRequest)
		return
	}

	// Reject the request if there's no admission request
	if admissionReview.Request == nil {
		log.Println("Request is nil")
		http.Error(w, "request is nil", http.StatusBadRequest)
		return
	}

	var response *admissionv1.AdmissionResponse
	if r.URL.Path == "/mutate" {
		response = si.mutate(&admissionReview)
	} else {
		response = &admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	admissionReview.Response = response
	resp, err := json.Marshal(admissionReview)
	if err != nil {
		log.Printf("Could not encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}

	log.Println("Webhook request handled successfully")
	if _, err := w.Write(resp); err != nil {
		log.Printf("Could not write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

// mutate handles the actual mutation of the deployment
func (si *SidecarInjector) mutate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	req := ar.Request

	// If not a deployment, skip mutation
	if req.Kind.Kind != "Deployment" {
		log.Printf("Not a deployment, skipping: %s", req.Kind.Kind)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: true,
		}
	}

	// Parse deployment from request
	var deployment appsv1.Deployment
	if err := json.Unmarshal(req.Object.Raw, &deployment); err != nil {
		log.Printf("Could not unmarshal raw object: %v", err)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// Check if the deployment has the required label
	enabled, ok := deployment.Labels[labelKey]
	if !ok || enabled != "true" {
		log.Printf("Deployment %s/%s does not have the required label, skipping", deployment.Namespace, deployment.Name)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: true,
		}
	}

	// Check if sidecar container already exists
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "websocket-proxy" {
			log.Printf("Sidecar already exists in deployment %s/%s, skipping", deployment.Namespace, deployment.Name)
			return &admissionv1.AdmissionResponse{
				UID:     req.UID,
				Allowed: true,
			}
		}
	}

	// Create a headless service name based on deployment name
	headlessServiceName := "ws-proxy-headless"

	// Create headless service
	headlessService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessServiceName,
			Namespace: deployment.Namespace,
			Labels: map[string]string{
				"app": deployment.Spec.Template.Labels["app"],
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None", // Makes it headless
			Selector:  deployment.Spec.Selector.MatchLabels,
			Ports: []corev1.ServicePort{
				{
					Port:       3000,
					TargetPort: intstr.FromInt(3000),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	// Get Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get cluster config: %v", err)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("Failed to create clientset: %v", err)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// Create the service
	_, err = clientset.CoreV1().Services(deployment.Namespace).Create(context.Background(), headlessService, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		failHeadlessCreationError := fmt.Errorf("Failed to create headless service: %w", err)
		log.Printf("Failed to create headless service: %v", failHeadlessCreationError)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: failHeadlessCreationError.Error(),
			},
		}
	}

	log.Printf("Injecting sidecar to deployment %s/%s", deployment.Namespace, deployment.Name)
	patch, err := createPatch(&deployment)
	if err != nil {
		log.Printf("Failed to create patch: %v", err)
		return &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	return &admissionv1.AdmissionResponse{
		UID:     req.UID,
		Allowed: true,
		Patch:   patch,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

// createPatch creates a JSON patch to add the sidecar container
func createPatch(deployment *appsv1.Deployment) ([]byte, error) {
	var patches []map[string]interface{}

	// Add sidecar container
	sidecarContainer := corev1.Container{
		Name:  "websocket-proxy",
		Image: "docker.io/lukas8219/websocket-operator-sidecar:latest", // Adjust image as needed
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 3000,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("25Mi"),
			},
		},
		EnvFrom: []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "websocket-operator-config",
					},
				},
			},
		},
	}

	containerPatch := map[string]interface{}{
		"op":    "add",
		"path":  "/spec/template/spec/containers/-",
		"value": sidecarContainer,
	}
	patches = append(patches, containerPatch)

	return json.Marshal(patches)
}

// getKeyPairFiles returns the paths to the TLS certificate and key files
func getKeyPairFiles() (string, string, error) {
	certFile := filepath.Join(certDir, "server.crt")
	keyFile := filepath.Join(certDir, "server.key")

	// Check if certificate files exist
	if _, err := os.Stat(certFile); err != nil {
		return "", "", fmt.Errorf("certificate file not found: %v", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return "", "", fmt.Errorf("key file not found: %v", err)
	}

	return certFile, keyFile, nil
}

func main() {
	log.Println("Starting webhook server")

	// Create the webhook server
	injector := NewSidecarInjector()
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", injector.sidecarInjectionHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start the server based on TLS configuration
	if tlsEnabled {
		certFile, keyFile, err := getKeyPairFiles()
		if err != nil {
			log.Printf("Warning: TLS certificate issue: %v", err)
			log.Println("Falling back to HTTP (not secure, for development only)")

			log.Printf("Listening on HTTP port %d", port)
			if err := server.ListenAndServe(); err != nil {
				log.Fatalf("Failed to start webhook server: %v", err)
			}
		} else {
			log.Printf("Listening on HTTPS port %d with TLS", port)
			if err := server.ListenAndServeTLS(certFile, keyFile); err != nil {
				log.Fatalf("Failed to start webhook server with TLS: %v", err)
			}
		}
	} else {
		log.Printf("TLS disabled, listening on HTTP port %d (not secure, for development only)", port)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Failed to start webhook server: %v", err)
		}
	}
}
