package route

import (
	"lukas8219/websocket-operator/internal/kubernetes"

	"lukas8219/websocket-operator/internal/dns"

	"github.com/dgryski/go-rendezvous"
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
		var sum uint64
		for _, c := range s {
			sum += uint64(c)
		}
		return sum
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
