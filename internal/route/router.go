package route

import (
	"lukas8219/websocket-operator/internal/kubernetes"

	"lukas8219/websocket-operator/internal/dns"

	"github.com/dgryski/go-rendezvous"

	"github.com/twmb/murmur3"
)

type RouterImpl interface {
	InitializeHosts() error
	Route(recipientId string) string
}

type RouterConfigMode string

type RouterConfig struct {
	Mode       RouterConfigMode
	ConfigMeta interface{}
}

const (
	RouterConfigModeDns        RouterConfigMode = "dns"
	RouterConfigModeKubernetes RouterConfigMode = "kubernetes"
)

func NewRouter(config RouterConfig) RouterImpl {

	rendezvous := rendezvous.New([]string{}, func(s string) uint64 {
		murmurAlgorithm := murmur3.New64()
		murmurAlgorithm.Write([]byte(s))
		hash := murmurAlgorithm.Sum64()
		return hash
	})
	switch config.Mode {
	case RouterConfigModeDns:
		return dns.WithDns(rendezvous)
	case RouterConfigModeKubernetes:
		return kubernetes.NewRouter(rendezvous)
	default:
		panic("invalid router mode")
	}
}
