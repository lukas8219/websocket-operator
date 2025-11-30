package peer_discovery_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"lukas8219/websocket-operator/internal/peer_discovery"
	"lukas8219/websocket-operator/internal/utils"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-set/v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestShouldHaveCorrectNumberOfHostsAfterReconcile(t *testing.T) {
	ctx := context.Background()
	kubeconfig := filepath.Join(
		os.Getenv("HOME"), ".kube", "config",
	)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	useConfig := true
	test := &envtest.Environment{
		UseExistingCluster: &useConfig,
		Config:             config,
	}
	env, err := test.Start()
	if err != nil {
		panic(err)
	}

	client := createClient(env)

	namespaceName := "kubernetes-peer-test"

	ns, error := client.CoreV1().Namespaces().Create(
		ctx,
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
		},
		metav1.CreateOptions{},
	)

	defer func() {
		client.CoreV1().Namespaces().Delete(
			ctx,
			ns.Name,
			metav1.DeleteOptions{},
		)
	}()

	var replicas int32 = 1
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-deployment",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas, // Helper function to get a pointer to int32
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-app",
							Image: "hashicorp/http-echo",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
							},
						},
					},
				},
			},
		}}

	// Create a headless service name based on deployment name
	headlessServiceName := "mock-headless"

	// Create headless service
	headlessService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      headlessServiceName,
			Namespace: ns.Name,
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

	_, err = client.AppsV1().Deployments(ns.Name).Create(
		ctx,
		deployment,
		metav1.CreateOptions{},
	)

	if err != nil {
		panic(err)
	}

	_, err = client.CoreV1().Services(ns.Name).Create(
		ctx,
		headlessService,
		metav1.CreateOptions{},
	)

	if err != nil {
		panic(err)
	}

	k8sPeer := peer_discovery.NewKubernetes(peer_discovery.KubernetesPeerDiscoveryOptions{
		Clientset: client,
		Namespace: ns.Name,
		Service:   headlessServiceName,
	})
	go k8sPeer.Initialize()
	var firstCall int32 = 5
	var secondCall int32 = 8
	var thirdCall int32 = 4
	var fourthCall int32 = 10

	scale := func(replicas int32) {
		time.Sleep(3 * time.Second)
		scaleDeployment(
			ctx,
			client,
			ns.Name,
			deployment.Name,
			replicas,
		)
	}

	scale(firstCall)
	scale(secondCall)
	scale(thirdCall)
	scale(fourthCall)

	error = waitBeforeAllPodsReady(ctx, client, ns.Name, deployment.Name, 30)
	must(error)

	time.Sleep(5 * time.Second)
	peers, error := k8sPeer.CurrentHosts()
	must(error)

	if len(peers) != int(fourthCall) {
		t.Fatalf("Peers has wrong number of hosts %s", peers)
	}

	if !hasPeersInEndpoint(
		ctx,
		client,
		ns.Name,
		headlessService.Name,
		peers,
	) {
		t.Fatal("Some peers are missing")
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func hasPeersInEndpoint(ctx context.Context, client *kubernetes.Clientset, ns string, service string, peers []peer_discovery.Peer) bool {
	endpoint, err := client.CoreV1().Endpoints(ns).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		slog.With("error", err).Error("Failed to fetch the Endpoint")
		return false
	}
	ips := set.From(utils.GetAllAddressesFromEndpoint(endpoint))
	peerIps := set.New[string](len(peers))
	for _, v := range peers {
		peerIps.Insert(v.Hostname())
	}
	return ips.EqualSet(peerIps)
}

func scaleDeployment(ctx context.Context, client *kubernetes.Clientset, ns string, deployment string, replicas int32) {
	currentScale, error := client.AppsV1().Deployments(ns).GetScale(
		ctx,
		deployment,
		metav1.GetOptions{},
	)
	must(error)
	currentScale.Spec.Replicas = replicas

	_, err := client.AppsV1().Deployments(ns).UpdateScale(
		ctx,
		deployment,
		currentScale,
		metav1.UpdateOptions{},
	)
	if err != nil {
		panic(err)
	}
}

func waitBeforeAllPodsReady(ctx context.Context, client *kubernetes.Clientset, ns string, deployment string, maxRetries uint16) error {
	_, error := doWithRetry(0, maxRetries, func() (bool, error) {
		data, error := client.AppsV1().Deployments(ns).Get(
			ctx,
			deployment,
			metav1.GetOptions{},
		)
		if error != nil {
			return false, error
		}
		fmt.Printf("Comparing %d to %d", *data.Spec.Replicas, data.Status.AvailableReplicas)
		return *data.Spec.Replicas == data.Status.AvailableReplicas, nil
	})

	return error
}

func doWithRetry(current, max uint16, fn func() (bool, error)) (bool, error) {
	if current >= max {
		return true, errors.New("Reached max attempts")
	}
	fmt.Printf("Try [%d] out of [%d]\n", current, max)
	done, err := fn()
	if done {
		return done, nil
	}
	if err != nil {
		return false, err
	}
	time.Sleep(5 * time.Second)
	return doWithRetry(current+1, max, fn)
}

// Remove OR MOVE
func createClient(config *rest.Config) *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		slog.Warn("Failed to get InClusterConfig, looking for KubeConfig", "error", err)
		kubeconfig := filepath.Join(
			os.Getenv("HOME"), ".kube", "config",
		)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	return kubernetes.NewForConfigOrDie(config)
}
