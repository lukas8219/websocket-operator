package kubernetes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"lukas8219/websocket-operator/internal/rendezvous"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	clientcmd "k8s.io/client-go/tools/clientcmd"
)

type KubernetesRouter struct {
	k8sClient    *kubernetes.Clientset
	cacheStore   cache.Store
	loadbalancer *rendezvous.Rendezvous
}

var (
	addedHosts = make(map[string]bool)
)

func NewRouter(loadbalancer *rendezvous.Rendezvous) *KubernetesRouter {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(
			os.Getenv("HOME"), ".kube", "config",
		)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		log.Println("Failed to get in-cluster config, using empty config")
	}

	client := kubernetes.NewForConfigOrDie(config)
	watchList := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "endpoints", "default",
		fields.OneTermEqualSelector("metadata.name", "ws-proxy-headless"),
	)
	store, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: watchList,
		ObjectType:    &v1.Endpoints{},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				hosts := make([]string, 1)
				for _, address := range obj.(*v1.Endpoints).Subsets {
					for _, address := range address.Addresses {
						hosts = append(hosts, address.IP)
					}
				}
				for _, host := range hosts {
					if loadbalancer.Lookup(host) == "" {
						loadbalancer.Add(host)
						addedHosts[host] = true
					}
				}
				log.Println("Added", hosts, "addresses")
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				hosts := make([]string, 1)
				//This is nuts, yes. But i'll look into re-writing the Rendezvous to be customized for this use case
				if len(addedHosts) > 0 {
					for _, subset := range oldObj.(*v1.Endpoints).Subsets {
						for _, address := range subset.Addresses {
							loadbalancer.Remove(address.IP)
							delete(addedHosts, address.IP)
						}
					}
				}
				for _, subset := range newObj.(*v1.Endpoints).Subsets {
					for _, address := range subset.Addresses {
						hosts = append(hosts, address.IP)
					}
				}
				for _, host := range hosts {
					loadbalancer.Add(host)
					addedHosts[host] = true
				}
				log.Println("Updated", hosts, "addresses")
			},
			DeleteFunc: func(obj interface{}) {
				hosts := make([]string, 1)
				for _, address := range obj.(*v1.Endpoints).Subsets[0].Addresses {
					hosts = append(hosts, address.IP)
				}
				for _, host := range hosts {
					loadbalancer.Remove(host)
				}
				log.Println("Deleted", hosts, "addresses")
			},
		},
	})
	stop := make(chan struct{})
	go controller.Run(stop)
	if !cache.WaitForCacheSync(stop, controller.HasSynced) {
		log.Fatal("Timed out waiting for caches to sync")
	}
	return &KubernetesRouter{
		k8sClient:    client,
		cacheStore:   store,
		loadbalancer: loadbalancer,
	}
}

func (k *KubernetesRouter) Route(recipientId string) string {
	host := k.loadbalancer.Lookup(recipientId)
	if host == "" {
		log.Println("No host found for", recipientId)
		return ""
	}
	log.Println("Host found for", recipientId, host)
	host = fmt.Sprintf("%s:3000", host)
	return host
}

func (k *KubernetesRouter) Add(host []string) {
	return
}

func (k *KubernetesRouter) InitializeHosts() error {
	endpoints := k.cacheStore.List()
	log.Println("Found", len(endpoints), "endpoints")
	hosts := make([]string, 0)
	//Yes lots of duplicate code
	for _, endpoint := range endpoints {
		for _, subset := range endpoint.(*v1.Endpoints).Subsets {
			for _, address := range subset.Addresses {
				hosts = append(hosts, address.IP)
			}
		}
	}
	for _, host := range hosts {
		k.loadbalancer.Add(host)
	}
	log.Println("Initialized", hosts, "hosts")
	return nil
}
