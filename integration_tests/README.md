# Kubernetes integration tests

These tests create a Deployment and headless Service in a uniquely named namespace,
scale through 1, 5, 8, 4, 10, and 0 replicas, and compare discovered peers with ready
Kubernetes endpoints at every step. Cleanup requests deletion of the test namespace.

The `Kubernetes integration` GitHub Actions workflow provisions a disposable kind
cluster for relevant pull requests and also supports manual dispatch. That workflow
has not been executed during this local preparation.

Use a dedicated test cluster and credentials allowed to create/delete namespaces and
manage Deployments, Services, and Endpoints. The cluster must be able to pull
`hashicorp/http-echo:1.0.0` and schedule ten small test pods.

```sh
# From the repository root, with Go 1.24.9 available:
export KUBECONFIG=/path/to/test-cluster.yaml
kubectl config current-context
make integration
```

The `integration` build tag is required. Missing credentials or failed readiness
checks fail the command; the test does not silently skip. `make test` and
`make test-race` run without a cluster. To run the cluster test with the race detector:

```sh
go test -race -tags=integration -count=1 -timeout=10m -v ./integration_tests/...
```

On a host that supports kind, a disposable cluster can be prepared with:

```sh
kind create cluster --name websocket-tests --image kindest/node:v1.34.0 \
  --kubeconfig /tmp/websocket-tests.kubeconfig
KUBECONFIG=/tmp/websocket-tests.kubeconfig make integration
kind delete cluster --name websocket-tests
```

This is an existing-cluster test: an API-server-only envtest instance is insufficient
because deployments, pods, and endpoints must be reconciled. The managed cloud
runtime used during PR preparation lacks cgroup delegation for rootless Docker, so
kind and the full integration test were not run there. Unit/race tests of peer-state
updates and compilation of the integration test can run without a cluster.

## Full-system deployment fixture

`scripts/test-system.sh` builds the controller, load balancer, sidecar, and demo
application from this checkout, loads them into a new kind cluster, installs the
TLS webhook, and verifies that the application receives exactly one sidecar using
the selected image. It uses its own temporary kubeconfig and removes the cluster
on exit. Requirements: Docker capable of running kind, kind, kubectl, openssl,
and jq. The `System integration` workflow runs it on GitHub-hosted runners.

An optional command runs after readiness, with `WS_E2E_URL` and
`WS_E2E_HOSTS_URL` pointing to the forwarded load-balancer listeners:

```sh
bash scripts/test-system.sh your-test-command its-arguments
```

This fixture checks deployment and injection. Message-delivery assertions are
supplied by the command, so deployment readiness alone is not a routing test.

Run the complete injection and message-delivery test with `make system-test`.
It waits for three peers, selects two distinct recipients on the same and a
separate pod, and verifies the demo application's exact reply through the load
balancer. It also checks rejection of a missing routing identity. The `e2e` build
tag keeps these tests separate from discovery-only and unit tests; running them
without the fixture's endpoint variables fails instead of silently skipping.

The full-system suite also opens concurrent clients, scales the application to
zero, requires HTTP 503 and disconnects for unroutable clients, restores three
replicas, and verifies a fresh connection can deliver again. Scaling commands
explicitly target the fixture-owned kind context, never the caller's current
context. Clients must reconnect after all peers disappear; the test does not
claim lossless migration of an in-flight message.
