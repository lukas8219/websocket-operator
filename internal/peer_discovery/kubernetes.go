package peer_discovery

import (
	"fmt"
	"log/slog"
	"lukas8219/websocket-operator/internal/diff"
	"os"
	"path/filepath"

	goset "github.com/hashicorp/go-set/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	clientcmd "k8s.io/client-go/tools/clientcmd"
)

type KubernetesPeerDiscovery struct {
	k8sClient            *kubernetes.Clientset
	cacheStore           cache.Store
	currentHosts         *goset.Set[string]
	targetK8sServiceName string
	k8sNamespace         string
	notificationChannel  chan diff.DifferenceOutput[Peer]
}

func NewKubernetes(namespace string, service string) PeerDiscovery {
	return &KubernetesPeerDiscovery{
		k8sNamespace:         namespace,
		targetK8sServiceName: service,
		k8sClient:            createClient(),
		currentHosts:         goset.New[string](100),
		notificationChannel:  make(chan diff.DifferenceOutput[Peer], 256),
	}
}

func (k *KubernetesPeerDiscovery) NotificationChannel() chan diff.DifferenceOutput[Peer] {
	return k.notificationChannel
}

var (
	EMPTY_ARRAY = make([]string, 0)
)

const (
	MAX_KUBERNETES_ENDPOINT_SLICE_SIZE = 1000
)

// Remove OR MOVE
func createClient() *kubernetes.Clientset {
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

func (k *KubernetesPeerDiscovery) Initialize() error {
	watchList := cache.NewListWatchFromClient(k.k8sClient.CoreV1().RESTClient(), "endpoints", k.k8sNamespace,
		fields.OneTermEqualSelector("metadata.name", k.targetK8sServiceName),
	)

	store, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: watchList,
		ObjectType:    &v1.Endpoints{},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				hosts := getAllAddressesFromEndpoint(obj.(*v1.Endpoints))
				k.updateHostsArray(hosts, EMPTY_ARRAY)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				hosts := getAllAddressesFromEndpoint(newObj.(*v1.Endpoints))
				k.updateHostsArray(hosts, EMPTY_ARRAY)
			},
			DeleteFunc: func(obj interface{}) {
				hosts := getAllAddressesFromEndpoint(obj.(*v1.Endpoints))
				k.updateHostsArray(EMPTY_ARRAY, hosts)
			},
		},
	})
	stop := make(chan struct{})
	go controller.Run(stop)
	if !cache.WaitForCacheSync(stop, controller.HasSynced) {
		slog.Error("Timed out waiting for caches to sync")
		return fmt.Errorf("timed out waiting for caches to sync")
	}
	k.cacheStore = store
	return nil
}

func getAllAddressesFromEndpoint(endpoint *v1.Endpoints) []string {
	hosts := make([]string, 0)
	for _, address := range endpoint.Subsets {
		for _, address := range address.Addresses {
			if address.IP != "" {
				hosts = append(hosts, address.IP)
			}
		}
	}
	return hosts
}

func (k *KubernetesPeerDiscovery) updateHostsArray(NewHosts []string, ToRemoveHosts []string) {
	difference := diff.Difference(k.currentHosts, NewHosts, ToRemoveHosts)
	k.notificationChannel <- diff.DifferenceOutput[Peer]{
		Added:   mapToPeers(difference.Added),
		Removed: mapToPeers(difference.Removed),
	}
}

func mapToPeers(hosts []string) []Peer {
	mappedHosts := make([]Peer, 0)
	for _, address := range hosts {
		mappedHosts = append(mappedHosts, Peer{
			hostname: address,
			port:     3000,
		})
	}
	return mappedHosts
}

func (k *KubernetesPeerDiscovery) CurrentHosts() ([]Peer, error) {
	mappedHosts := mapToPeers(k.currentHosts.Slice())
	return mappedHosts, nil
}

func (k *KubernetesPeerDiscovery) Mode() PeerDiscoveryMode {
	return PeerDiscoveryModeKubernetes
}
