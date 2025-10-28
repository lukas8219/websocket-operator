package peer_discovery

import (
	"fmt"
	"log/slog"
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
	currentHosts         goset.Set[string]
	targetK8sServiceName string
	k8sNamespace         string
	PeerDiscovery
}

func NewKubernetes(namespace string, service string) *KubernetesPeerDiscovery {
	return &KubernetesPeerDiscovery{
		k8sNamespace:         namespace,
		targetK8sServiceName: service,
	}
}

// Remove OR MOVE
func createClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(
			os.Getenv("HOME"), ".kube", "config",
		)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		slog.Info("Failed to get in-cluster config, using empty config")
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
				hosts := make([]string, 0)
				for _, address := range obj.(*v1.Endpoints).Subsets {
					for _, address := range address.Addresses {
						if address.IP != "" {
							hosts = append(hosts, address.IP)
						}
					}
				}
				k.currentHosts.InsertSlice(hosts)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				//pre-allocate more before hand
				hosts := make([]string, 0)
				//This is nuts, yes. But i'll look into re-writing the Rendezvous to be customized for this use case
				for _, subset := range newObj.(*v1.Endpoints).Subsets {
					for _, address := range subset.Addresses {
						hosts = append(hosts, address.IP)
					}
				}
				k.currentHosts.InsertSlice(hosts)
			},
			DeleteFunc: func(obj interface{}) {
				hosts := make([]string, 1)
				for _, address := range obj.(*v1.Endpoints).Subsets[0].Addresses {
					hosts = append(hosts, address.IP)
				}
				k.currentHosts.RemoveSlice(hosts)
				slog.Info("Deleted addresses", "hosts", hosts)
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

func (k *KubernetesPeerDiscovery) GetCurrentHosts() ([]Peer, error) {
	mappedHosts := make([]Peer, k.currentHosts.Size())
	for address := range k.currentHosts.Items() {
		mappedHosts = append(mappedHosts, Peer{
			hostname: address,
			port:     "3000",
		})
	}
	return mappedHosts, nil
}
