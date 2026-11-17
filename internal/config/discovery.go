package config

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/util/validation"
)

type Discovery struct {
	Namespace string
	Service   string
}

// DiscoveryFromEnv keeps local defaults while allowing namespace-scoped installs.
func DiscoveryFromEnv() (Discovery, error) {
	result := Discovery{Namespace: "default", Service: "ws-proxy-headless"}
	for name, target := range map[string]*string{
		"WS_OPERATOR_NAMESPACE": &result.Namespace,
		"WS_OPERATOR_SERVICE":   &result.Service,
	} {
		if value, present := os.LookupEnv(name); present {
			if problems := validation.IsDNS1123Label(value); len(problems) != 0 {
				return Discovery{}, fmt.Errorf("%s: %v", name, problems)
			}
			*target = value
		}
	}
	return result, nil
}
