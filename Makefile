COMPONENTS := sidecar controller loadbalancer

.PHONY: all gen-certs test test-race
.DEFAULT_GOAL := all
all: $(addprefix build-,$(COMPONENTS))

build-%:
	@echo "Building WebSocket $*"
	COMPONENT="$*" ./scripts/build.sh

push-%:
	@echo "Build and Push Image $*"
	COMPONENT="$*" PUSH="true" ./scripts/build.sh

gen-certs:
	@echo "Generating TLS certificates..."
	./scripts/gen-certs.sh

test:
	@echo "Running tests..."
	go test -v ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...
