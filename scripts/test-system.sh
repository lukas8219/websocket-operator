#!/usr/bin/env bash
# Own a disposable kind cluster; never modify the caller's active kubeconfig.
set -euo pipefail
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"
for command in docker kind kubectl openssl jq; do command -v "$command" >/dev/null; done
system_tmp=$(mktemp -d)
cluster="websocket-e2e-$$"
image_tag="e2e-$(git rev-parse --short HEAD)"
export KUBECONFIG="$system_tmp/kubeconfig"
install_mode="${WS_E2E_INSTALL:-manifests}"
namespace=default
controller_namespace=websocket-system
controller=websocket-operator-controller
loadbalancer=websocket-operator-loadbalancer
application_account=loadbalancer-sa
case "$install_mode" in
  manifests) ;;
  helm)
    command -v helm >/dev/null
    namespace=websocket-chart
    controller_namespace="$namespace"
    controller=chat-ws-ctrl
    loadbalancer=chat-ws-lb
    application_account=default ;;
  *) echo "Unknown install mode: $install_mode" >&2; exit 1 ;;
esac
export WS_E2E_NAMESPACE="$namespace"
export WS_E2E_USER_HEADER="${WS_OPERATOR_USER_HEADER:-ws-user-id}"
created=0
forward_pid=
cleanup() {
  status=$?
  if (( status != 0 && created )); then
    kubectl get pods -A -o wide || true
    # A restart kills kubectl port-forward. Preserve the terminated process's
    # reason and logs; logs from its replacement cannot explain the failure.
    kubectl get pods -n "$namespace" -o json | jq -c '
      .items[] | .metadata.name as $pod | .status.containerStatuses[]? |
      select(.lastState.terminated != null) |
      {pod:$pod, container:.name, lastTermination:.lastState.terminated}' || true
    kubectl logs -n "$namespace" "deployment/$loadbalancer" --previous --tail=100 || true
    kubectl logs -n "$controller_namespace" "deployment/$controller" --tail=60 || true
    kubectl logs "deployment/$loadbalancer" --tail=60 || true
    kubectl logs -l app=main-ws-server --all-containers --tail=40 --prefix || true
  fi
  if [[ -n "$forward_pid" ]]; then kill "$forward_pid" 2>/dev/null || true; wait "$forward_pid" 2>/dev/null || true; fi
  if (( created )); then kind delete cluster --name "$cluster"; fi
  rm -rf "$system_tmp"
  exit "$status"
}
trap cleanup EXIT
# Refuse to reuse an existing cluster, even if a name happens to collide.
if kind get clusters | awk -v name="$cluster" '$0 == name { found=1 } END { exit !found }'; then
  printf 'Cluster already exists: %s\n' "$cluster" >&2; exit 1
fi
for component in controller loadbalancer sidecar; do
  docker build -f "cmd/$component/Dockerfile" -t "websocket-operator-$component:$image_tag" .
done
docker build -f deployments/local/Dockerfile -t "websocket-operator-test-server:$image_tag" deployments/local
kind create cluster --name "$cluster" --image kindest/node:v1.34.0 --kubeconfig "$KUBECONFIG" --wait 120s
created=1
for component in controller loadbalancer sidecar test-server; do
  kind load docker-image --name "$cluster" "websocket-operator-$component:$image_tag"
done
kubectl config set-context --current --namespace="$namespace"
if [[ "$install_mode" == helm ]]; then
  source scripts/install-system-helm.sh
else
CERT_DIR="$system_tmp/certs" MANIFEST_DIR="$system_tmp/manifests" bash scripts/gen-certs.sh
kubectl apply -f deployments/k8s/namespace.yaml
kubectl create --dry-run=client -f deployments/k8s/configmap.yaml -o json |
  jq --arg header "$WS_E2E_USER_HEADER" ' .data.WS_OPERATOR_USER_HEADER = $header' | kubectl apply -f -
kubectl apply -f deployments/k8s/rbac.yaml
kubectl apply -f "$system_tmp/manifests/tls-secrets-generated.yaml" -f deployments/k8s/service.yaml
# Render only the development image/arguments overrides; preserve the shipped resources.
kubectl create --dry-run=client -f deployments/k8s/deployment.yaml -o json |
  jq -s --arg tag "$image_tag" '
    [.[] | if .kind == "List" then .items[] else . end] |
    {apiVersion:"v1", kind:"List", items: map(if .kind == "Deployment" then
      .spec.template.spec.containers[0] |= (
        .image = ("websocket-operator-" + .name + ":" + $tag) |
        .imagePullPolicy = "IfNotPresent" |
        if .name == "controller" then .args += ["-sidecar-image=websocket-operator-sidecar:" + $tag] else . end)
    else . end)}' | kubectl apply -f -
kubectl -n websocket-system rollout status deployment/websocket-operator-controller --timeout=120s
kubectl rollout status deployment/websocket-operator-loadbalancer --timeout=120s
kubectl apply -f "$system_tmp/manifests/webhook-generated.yaml"
fi
kubectl create --dry-run=client -f deployments/k8s/target-deploy.yaml -o json |
  jq --arg image "websocket-operator-test-server:$image_tag" --arg namespace "$namespace" --arg account "$application_account" '
    .metadata.namespace = $namespace |
    .spec.template.spec.serviceAccountName = $account |
    .spec.replicas = 3 |
    .spec.template.spec.containers[0] |= (
      .image = $image | .imagePullPolicy = "IfNotPresent" |
      .readinessProbe = {tcpSocket:{port:3001},periodSeconds:1})' | kubectl apply -f -
kubectl rollout status deployment/main-ws-server --timeout=180s
kubectl get deployment main-ws-server -o json | jq -e --arg image "websocket-operator-sidecar:$image_tag" '
  [.spec.template.spec.containers[] | select(.name == "websocket-proxy" and .image == $image)] | length == 1'
kubectl port-forward "deployment/$loadbalancer" --address=127.0.0.1 0:8080 0:8081 > "$system_tmp/ports.log" 2>&1 &
forward_pid=$!
deadline=$((SECONDS + 30))
while (( SECONDS < deadline )); do
  ws_port=$(awk '/Forwarding from 127.0.0.1:.* -> 8080/ {split($3,a,":");print a[2];exit}' "$system_tmp/ports.log")
  hosts_port=$(awk '/Forwarding from 127.0.0.1:.* -> 8081/ {split($3,a,":");print a[2];exit}' "$system_tmp/ports.log")
  if [[ -n "$ws_port" && -n "$hosts_port" ]]; then break; fi
  kill -0 "$forward_pid" || { cat "$system_tmp/ports.log"; exit 1; }
  sleep 0.2
done
[[ -n "$ws_port" && -n "$hosts_port" ]] || { cat "$system_tmp/ports.log"; exit 1; }
export WS_E2E_CLUSTER="$cluster"
export WS_E2E_URL="ws://127.0.0.1:$ws_port"
export WS_E2E_HOSTS_URL="http://127.0.0.1:$hosts_port"
printf 'Injected deployment ready; WebSocket endpoint: %s\n' "$WS_E2E_URL"
if (( $# )); then "$@"; fi

if [[ "$install_mode" == helm ]]; then
  kubectl delete deployment main-ws-server --wait=true
  helm uninstall chat --namespace "$namespace" --wait --timeout 2m
  [[ -z "$(kubectl get service chat-ws-peers --ignore-not-found -o name)" ]]
  [[ -z "$(kubectl get secret chat-ws-tls --ignore-not-found -o name)" ]]
  [[ -z "$(kubectl get mutatingwebhookconfiguration "chat-ws-$namespace" --ignore-not-found -o name)" ]]
fi
