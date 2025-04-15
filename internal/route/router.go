package route

import (
	"log/slog"
	"lukas8219/websocket-operator/internal/kubernetes"

	"lukas8219/websocket-operator/internal/dns"

	"lukas8219/websocket-operator/internal/rendezvous"
)

type Logger interface {
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
	Error(msg string, args ...any)
}

type RouterImpl interface {
	InitializeHosts() error
	Route(recipientId string) string
	RebalanceRequests() <-chan [][2]string
	Logger
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
