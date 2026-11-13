//go:build integration

package peer_discovery_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-set/v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/utils"
)

func TestShouldHaveCorrectNumberOfHostsAfterReconcile(t *testing.T) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatalf("load test-cluster kubeconfig (KUBECONFIG or default path): %v", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ns, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "websocket-peer-test-",
			Labels:       map[string]string{"websocket-operator-injection": "disabled"},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create isolated test namespace: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := client.CoreV1().Namespaces().Delete(cleanupCtx, ns.Name, metav1.DeleteOptions{}); err != nil {
			t.Errorf("delete namespace %s: %v", ns.Name, err)
		}
	})

	replicas := int32(1)
	labels := map[string]string{"app": "peer-discovery-test"}
	deployment, err := client.AppsV1().Deployments(ns.Name).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deployment"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: "echo", Image: "hashicorp/http-echo:1.0.0",
					Args:  []string{"-listen=:8080", "-text=peer-discovery"},
					Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler:  corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromInt(8080)}},
						PeriodSeconds: 1,
					},
				}}},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	service, err := client.CoreV1().Services(ns.Name).Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "mock-headless"},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone, Selector: labels,
			Ports: []corev1.ServicePort{{Port: 3000, TargetPort: intstr.FromInt(8080)}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create headless service: %v", err)
	}
	discovery := peer_discovery.NewKubernetes(peer_discovery.KubernetesPeerDiscoveryOptions{
		Clientset: client, Namespace: ns.Name, Service: service.Name,
	})
	initialized := make(chan error, 1)
	go func() { initialized <- discovery.Initialize() }()
	select {
	case err := <-initialized:
		if err != nil {
			t.Fatalf("initialize peer discovery: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("initialize peer discovery: %v", ctx.Err())
	}
	// Drain notifications as the resolver would, including scale-to-zero updates.
	go func() {
		for {
			select {
			case <-discovery.NotificationChannel():
			case <-ctx.Done():
				return
			}
		}
	}()
	for _, desired := range []int32{1, 5, 8, 4, 10, 0} {
		if !t.Run(fmt.Sprintf("replicas_%d", desired), func(t *testing.T) {
			scale, err := client.AppsV1().Deployments(ns.Name).GetScale(ctx, deployment.Name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			scale.Spec.Replicas = desired
			if _, err = client.AppsV1().Deployments(ns.Name).UpdateScale(ctx, deployment.Name, scale, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			var observed string
			err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 90*time.Second, true, func(ctx context.Context) (bool, error) {
				current, err := client.AppsV1().Deployments(ns.Name).Get(ctx, deployment.Name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				endpoint, err := client.CoreV1().Endpoints(ns.Name).Get(ctx, service.Name, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				peers, err := discovery.CurrentHosts()
				if err != nil {
					return false, err
				}
				addresses := set.From(utils.GetAllAddressesFromEndpoint(endpoint))
				peerAddresses := set.New[string](len(peers))
				for _, peer := range peers {
					peerAddresses.Insert(peer.Hostname())
				}
				observed = fmt.Sprintf("ready=%d endpoints=%d peers=%d", current.Status.ReadyReplicas, addresses.Size(), peerAddresses.Size())
				return current.Status.ObservedGeneration >= current.Generation &&
					current.Status.ReadyReplicas == desired && addresses.Size() == int(desired) &&
					peerAddresses.EqualSet(addresses), nil
			})
			if err != nil {
				t.Fatalf("waiting for %d replicas (%s): %v", desired, observed, err)
			}
		}) {
			break
		}
	}
}
