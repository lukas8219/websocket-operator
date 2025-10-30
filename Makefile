COMPONENTS := sidecar controller loadbalancer

.PHONY: all gen-certs test test-race
.DEFAULT_GOAL := all
all: $(addprefix build-,$(COMPONENTS))


build-%:
	@echo "Building WebSocket $*"
	COMPONENT="$*" ./scripts/build.sh

push-all: $(addprefix push-,$(COMPONENTS))
push-%:
	@echo "Build and Push Image $*"
	COMPONENT="$*" PUSH="true" ./scripts/build.sh

run-%:
	go run "./cmd/$*/"

test-server-push:
	docker build -f deployments/local/Dockerfile -t lukas8219/websocket-operator-test-server:latest
	docker push -t lukas8219/websocket-operator-test-server:latest

gen-certs:
	@echo "Generating TLS certificates..."
	./scripts/gen-certs.sh

test:
	@echo "Running tests..."
	go test -v ./...

integration:
	go test -v ./integration_tests/...

test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...
