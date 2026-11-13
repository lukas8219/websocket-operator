COMPONENTS := sidecar controller loadbalancer

.PHONY: all build push gen-certs test test-race test-server-push integration
.DEFAULT_GOAL := all
all: $(addprefix build-,$(COMPONENTS))


build-%:
	@echo "Building WebSocket $*"
	COMPONENT="$*" ./scripts/build.sh

push-%:
	@echo "Build and Push Image $*"
	COMPONENT="$*" PUSH="true" ./scripts/build.sh

push: $(addprefix push-,$(COMPONENTS))
build: $(addprefix build-,$(COMPONENTS))

run-%:
	go run "./cmd/$*/"

test-server-push:
	docker build -f deployments/local/Dockerfile -t lukas8219/websocket-operator-test-server:latest deployments/local
	docker push lukas8219/websocket-operator-test-server:latest

gen-certs:
	@echo "Generating TLS certificates..."
	./scripts/gen-certs.sh

test:
	@echo "Running tests..."
	go test -v ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...

# Requires a dedicated test cluster selected by KUBECONFIG.
integration:
	go test -tags=integration -count=1 -timeout=10m -v ./integration_tests/...
