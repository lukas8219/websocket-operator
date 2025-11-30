package utils

import v1 "k8s.io/api/core/v1"

func GetAllAddressesFromEndpoint(endpoint *v1.Endpoints) []string {
	hosts := make([]string, 0)
	for _, address := range endpoint.Subsets {
		for _, address := range address.Addresses {
			if address.IP != "" {
				hosts = append(hosts, address.IP)
			}
		}
	}
	return hosts
}
