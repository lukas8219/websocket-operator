package kubernetes

import (
	"fmt"
	"log/slog"
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
	rebalancedHostTrigger       []func([][2]string) error
	alreadyCalculatedRecipients map[string]string
	handleUpdatedEndpoints      func([]string)
	handleCreatedEnpoints       func([]string)
	handleDeletedEnpoints       func([]string)
}

func (k *KubernetesRouter) Info(msg string, args ...any) {
	slog.With("component", "router").With("mode", "kubernetes").Info(msg, args...)
}

func (k *KubernetesRouter) Debug(msg string, args ...any) {
	slog.With("component", "router").With("mode", "kubernetes").Debug(msg, args...)
}

func (k *KubernetesRouter) Error(msg string, args ...any) {
	slog.With("component", "router").With("mode", "kubernetes").Error(msg, args...)
}

func (k *KubernetesRouter) GetAllUpstreamHosts() []string {
	return k.loadbalancer.GetAllHosts()
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
		rebalancedHostTrigger:       make([]func([][2]string) error, 0),
	}
}

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

func (k *KubernetesRouter) Route(recipientId string) string {
	k.Debug("Lookup", "recipientId", recipientId, "nodes", k.loadbalancer.GetNodes())
	host := k.loadbalancer.Lookup(recipientId)
	if host == "" {
		k.Debug("No host found", "recipientId", recipientId, "nodes", k.loadbalancer.GetNodes())
		return ""
	}
	k.alreadyCalculatedRecipients[recipientId] = host
	k.Debug("Host found", "recipientId", recipientId, "host", host)
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
				hosts := make([]string, 0)
				for _, address := range obj.(*v1.Endpoints).Subsets {
					for _, address := range address.Addresses {
						if address.IP != "" {
							hosts = append(hosts, address.IP)
						}
					}
				}
				for _, host := range hosts {
					k.loadbalancer.Add(host)
					addedHosts[host] = true
				}
				k.Info("Added addresses", "hosts", hosts)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				hosts := make([]string, 0)
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
				k.Info("Updated addresses", "hosts", hosts)
				rebalanceHosts := make([][2]string, 0)
				//re-calculate computed recipients to check re-balancing
				k.Debug("Already calculated recipients", "recipients", k.alreadyCalculatedRecipients)
				for recipientId, host := range k.alreadyCalculatedRecipients {
					newlyCalculatedHost := k.loadbalancer.Lookup(recipientId)
					k.Debug("Checking rebalance", "recipientId", recipientId, "oldHost", host, "newHost", newlyCalculatedHost)
					if newlyCalculatedHost != host {
						hostWithPort := fmt.Sprintf("%s:3000", host)
						newlyCalculatedHostWithPort := fmt.Sprintf("%s:3000", newlyCalculatedHost)
						rebalanceHosts = append(rebalanceHosts, [2]string{hostWithPort, newlyCalculatedHostWithPort})
					}
				}
				if len(rebalanceHosts) > 0 {
					k.Info("Rebalancing hosts", "hosts", rebalanceHosts)
					k.triggerRebalance(rebalanceHosts)
				} else {
					k.Debug("No rebalancing hosts found")
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
				k.Info("Deleted addresses", "hosts", hosts)
			},
		},
	})
	stop := make(chan struct{})
	go controller.Run(stop)
	if !cache.WaitForCacheSync(stop, controller.HasSynced) {
		k.Error("Timed out waiting for caches to sync")
		return fmt.Errorf("timed out waiting for caches to sync")
	}
	k.cacheStore = store
	return nil
}

func (k *KubernetesRouter) triggerRebalance(hosts [][2]string) error {
	if k.rebalancedHostTrigger == nil {
		k.Debug("No rebalance trigger found")
		return nil
	}

	for _, fn := range k.rebalancedHostTrigger {
		k.Debug("Triggering rebalance", "hosts", hosts)
		err := fn(hosts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *KubernetesRouter) OnHostRebalance(fn func([][2]string) error) {
	k.rebalancedHostTrigger = append(k.rebalancedHostTrigger, fn)
}
