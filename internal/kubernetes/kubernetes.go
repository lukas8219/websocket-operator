package kubernetes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dgryski/go-rendezvous"
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
				hosts := make([]string, 0)
				for _, address := range obj.(*v1.Endpoints).Subsets[0].Addresses {
					hosts = append(hosts, address.IP)
				}
				for _, host := range hosts {
					loadbalancer.Remove(host)
					loadbalancer.Add(host)
				}
				log.Println("Added", hosts, "addresses")
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				hosts := make([]string, 0)
				for _, address := range newObj.(*v1.Endpoints).Subsets[0].Addresses {
					hosts = append(hosts, address.IP)
				}
				for _, host := range hosts {
					loadbalancer.Remove(host)
					loadbalancer.Add(host)
				}
				log.Println("Updated", hosts, "addresses")
			},
			DeleteFunc: func(obj interface{}) {
				hosts := make([]string, 0)
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
	if err != nil {
		log.Println("Failed to resync cache", err)
	}
	return &KubernetesRouter{
		k8sClient:    client,
		cacheStore:   store,
		loadbalancer: loadbalancer,
	}
}

func (k *KubernetesRouter) Route(recipientId string) string {
	return fmt.Sprintf("%s:3000", k.loadbalancer.Lookup(recipientId))
}

func (k *KubernetesRouter) Add(host []string) {
	return
}

func (k *KubernetesRouter) InitializeHosts() error {
	endpoints := k.cacheStore.List()
	log.Println("Found", len(endpoints), "endpoints")
	hosts := make([]string, 0)
	for _, endpoint := range endpoints {
		for _, address := range endpoint.(*v1.Endpoints).Subsets[0].Addresses {
			hosts = append(hosts, fmt.Sprintf("%s:3000", address.IP))
		}
	}
	for _, host := range hosts {
		k.loadbalancer.Add(host)
	}
	log.Println("Initialized", hosts, "hosts")
	return nil
}
