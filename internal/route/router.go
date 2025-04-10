package route

import (
	"log/slog"
	"lukas8219/websocket-operator/internal/kubernetes"

	"lukas8219/websocket-operator/internal/dns"

	"lukas8219/websocket-operator/internal/rendezvous"
)

type RouterImpl interface {
	InitializeHosts() error
	Route(recipientId string) string
	OnHostRebalance(func([][2]string) error)
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
	slog.With("component", "router").With("mode", config.Mode).Info("New router")
	rendezvous := rendezvous.NewDefault()
	switch config.Mode {
	case RouterConfigModeDns:
		return dns.WithDns(rendezvous)
	case RouterConfigModeKubernetes:
		return kubernetes.NewRouter(rendezvous)
	default:
		panic("invalid router mode")
	}
}
