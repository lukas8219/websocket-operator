.PHONY: all build-sidecar build-controller gen-certs

all: build-sidecar build-controller gen-certs

build-sidecar:
	@echo "Building WebSocket Proxy Sidecar..."
	./scripts/build-sidecar.sh

build-controller:
	@echo "Building WebSocket Operator Controller..."
	./scripts/build-controller.sh
	
build-loadbalancer:
	@echo "Building WebSocket Operator LoadBalancer..."
	./scripts/build-loadbalancer.sh

gen-certs:
	@echo "Generating TLS certificates..."
	./scripts/gen-certs.sh

