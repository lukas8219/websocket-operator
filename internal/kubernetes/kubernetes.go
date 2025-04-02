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
	k8sClient                   *kubernetes.Clientset
	cacheStore                  cache.Store
	loadbalancer                *rendezvous.Rendezvous
	rebalancedHostTrigger       []func([]string) error
	alreadyCalculatedRecipients map[string]string
	handleUpdatedEndpoints      func([]string)
	handleCreatedEnpoints       func([]string)
	handleDeletedEnpoints       func([]string)
}

var (
	addedHosts = make(map[string]bool)
)

func NewRouter(loadbalancer *rendezvous.Rendezvous) *KubernetesRouter {
	client := createClient()
	return &KubernetesRouter{
		k8sClient:                   client,
		alreadyCalculatedRecipients: make(map[string]string),
		loadbalancer:                loadbalancer,
		rebalancedHostTrigger:       make([]func([]string) error, 0),
	}
}

func createClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := filepath.Join(
			os.Getenv("HOME"), ".kube", "config",
		)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		log.Println("Failed to get in-cluster config, using empty config")
	}

	return kubernetes.NewForConfigOrDie(config)
}

func (k *KubernetesRouter) Route(recipientId string) string {
	host := k.loadbalancer.Lookup(recipientId)
	if host == "" {
		log.Println("No host found for", recipientId, "out of", k.loadbalancer.GetNodes())
		return ""
	}
	k.alreadyCalculatedRecipients[recipientId] = host
	log.Println("Host found for", recipientId, host)
	host = fmt.Sprintf("%s:3000", host)
	return host
}

func (k *KubernetesRouter) Add(host []string) {
	return
}

func (k *KubernetesRouter) InitializeHosts() error {
	watchList := cache.NewListWatchFromClient(k.k8sClient.CoreV1().RESTClient(), "endpoints", "default",
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
					if k.loadbalancer.Lookup(host) == "" {
						k.loadbalancer.Add(host)
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
							k.loadbalancer.Remove(address.IP)
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
					k.loadbalancer.Add(host)
					addedHosts[host] = true
				}
				log.Println("Updated", hosts, "addresses")
				rebalanceHosts := make([]string, 0)
				//re-calculate computed recipients to check re-balancing
				for recipientId, _ := range k.alreadyCalculatedRecipients {
					rebalanceHosts = append(rebalanceHosts, recipientId)
				}
				if len(rebalanceHosts) > 0 {
					k.triggerRebalance(rebalanceHosts)
				}
			},
			DeleteFunc: func(obj interface{}) {
				hosts := make([]string, 1)
				for _, address := range obj.(*v1.Endpoints).Subsets[0].Addresses {
					hosts = append(hosts, address.IP)
				}
				for _, host := range hosts {
					k.loadbalancer.Remove(host)
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
	k.cacheStore = store
	return nil
}

func (k *KubernetesRouter) triggerRebalance(hosts []string) error {
	if k.rebalancedHostTrigger == nil {
		return nil
	}

	for _, fn := range k.rebalancedHostTrigger {
		err := fn(hosts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *KubernetesRouter) OnHostRebalance(fn func([]string) error) {
	k.rebalancedHostTrigger = append(k.rebalancedHostTrigger, fn)
}
