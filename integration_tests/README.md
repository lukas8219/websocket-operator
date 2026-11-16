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
