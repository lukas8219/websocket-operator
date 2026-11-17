# WebSocket admission controller

The controller watches admission requests for Deployments. A Deployment with
`metadata.labels["ws.operator/enabled"]: "true"` receives a `websocket-proxy`
container unless it already has one. It also creates `ws-proxy-headless` in the
Deployment's namespace. The target application must listen on port 3001.

## Build

Run from the **repository root**:

```sh
make build-controller
# Optional publishing, using credentials for your registry:
# REGISTRY=docker.io REPOSITORY=your-account TAG=dev make push-controller
```

There are no component-local `build.sh` or `deploy.sh` scripts. The shared build
entry point is `scripts/build.sh`, invoked by the Makefile.

## Development deployment

Use a dedicated Kubernetes cluster. The example manifests reference
`docker.io/lukas8219/websocket-operator-{controller,loadbalancer}:latest`; the
controller injects `docker.io/lukas8219/websocket-operator-sidecar:latest` by default.
Set its `-sidecar-image` flag to inject the version you built.
Check that those images contain the revision you intend to run. Building an image
locally does not publish it or make it available to cluster nodes.

From the repository root, with `openssl` and `kubectl` installed:

```sh
make gen-certs
kubectl apply -f deployments/k8s/namespace.yaml
kubectl apply -f deployments/k8s/configmap.yaml -f deployments/k8s/rbac.yaml
kubectl apply -f deployments/k8s/tls-secrets-generated.yaml
kubectl apply -f deployments/k8s/service.yaml -f deployments/k8s/deployment.yaml
kubectl -n websocket-system rollout status deployment/websocket-operator-controller
kubectl apply -f deployments/k8s/webhook-generated.yaml
kubectl apply -f deployments/k8s/target-deploy.yaml
```

Install the webhook **before** the labeled application Deployment so admission
can inject the sidecar. The target manifest also supplies the application label,
service account, and port. Inspect injection with
`kubectl get deployment main-ws-server -o jsonpath='{.spec.template.spec.containers[*].name}'`.

The certificate script creates a development CA, a certificate for the webhook
Service DNS names, and rendered Secret/webhook manifests. Generated files are
ignored by Git. Re-running it rotates certificates; apply both generated manifests
together. The webhook calls the controller over HTTPS on Service port 443.

For troubleshooting, check the controller rollout, its logs, webhook CA bundle,
and the exact `ws.operator/enabled` label. The `/health` endpoint uses the same
HTTP/TLS listener as `/mutate`. Controller flags include `-port`, `-cert-dir`, and
`-tls-enabled`; disabling TLS is only useful when calling the process directly,
not through Kubernetes' HTTPS admission webhook.
