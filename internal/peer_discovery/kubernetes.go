package peer_discovery

import (
	"fmt"
	"log/slog"
	"lukas8219/websocket-operator/internal/diff"
	"lukas8219/websocket-operator/internal/utils"

	goset "github.com/hashicorp/go-set/v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type KubernetesPeerDiscoveryOptions struct {
	Clientset *kubernetes.Clientset
	Namespace string
	Service   string
}

type KubernetesPeerDiscovery struct {
	k8sClient            *kubernetes.Clientset
	cacheStore           cache.Store
	currentHosts         *goset.Set[string]
	targetK8sServiceName string
	k8sNamespace         string
	notificationChannel  chan diff.DifferenceOutput[Peer]
}

func NewKubernetes(options KubernetesPeerDiscoveryOptions) PeerDiscovery {
	return &KubernetesPeerDiscovery{
		k8sNamespace:         options.Namespace,
		targetK8sServiceName: options.Service,
		k8sClient:            options.Clientset,
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

func (k *KubernetesPeerDiscovery) Initialize() error {
	watchList := cache.NewListWatchFromClient(k.k8sClient.CoreV1().RESTClient(), "endpoints", k.k8sNamespace,
		fields.OneTermEqualSelector("metadata.name", k.targetK8sServiceName),
	)

	store, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: watchList,
		ObjectType:    &v1.Endpoints{},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				hosts := utils.GetAllAddressesFromEndpoint(obj.(*v1.Endpoints))
				k.updateHostsArray(hosts, EMPTY_ARRAY)
			},
			//TODO when scaling up to 20 replicas, the current state was a single entry in CURRENT_HOSTS
			UpdateFunc: func(oldObj, newObj interface{}) {
				hosts := utils.GetAllAddressesFromEndpoint(newObj.(*v1.Endpoints))
				oldHosts := utils.GetAllAddressesFromEndpoint(oldObj.(*v1.Endpoints))
				k.updateHostsArray(hosts, oldHosts)
			},
			DeleteFunc: func(obj interface{}) {
				hosts := utils.GetAllAddressesFromEndpoint(obj.(*v1.Endpoints))
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

func (k *KubernetesPeerDiscovery) updateHostsArray(NewHosts []string, ToRemoveHosts []string) {
	beforeState := k.currentHosts.Copy().Slice()
	difference := diff.Difference(k.currentHosts, NewHosts, ToRemoveHosts)
	if k.currentHosts.Size() == 0 {
		return
	}
	added, removed := mapToPeers(difference.Added), mapToPeers(difference.Removed)
	slog.With("added", added, "removed", removed, "new", NewHosts, "toDelete", ToRemoveHosts, "before", mapToPeers(beforeState)).Info("New Host State Diff")
	k.notificationChannel <- diff.DifferenceOutput[Peer]{
		Added:   added,
		Removed: removed,
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
