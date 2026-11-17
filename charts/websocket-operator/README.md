# WebSocket operator chart

Install one routing pool per namespace using Helm 3. The chart manages the
controller, load balancer, discovery Service, namespace-scoped RBAC and TLS webhook.
The application Deployment is managed separately.

Build/publish all three project images from the same revision, then select that
tag. The public `latest` images may predate the configuration flags used here.

```sh
helm upgrade --install chat charts/websocket-operator --namespace chat --create-namespace \
  --set images.tag=YOUR_REVISION --wait --timeout 3m
kubectl -n chat apply -f YOUR_APPLICATION_DEPLOYMENT.yaml
kubectl -n chat port-forward service/chat-ws-lb 8080:8080
```

The application's **Deployment metadata** needs `ws.operator/enabled: "true"`.
Its pod labels must match `application.selector` (default `app: main-ws-server`),
and its WebSocket server must listen on port 3001. Use
`application.serviceAccount` to grant discovery access to an existing application
ServiceAccount. Keep the Deployment and all operator components in the release
namespace. Multiple independent application pools in that namespace are unsupported.

Configure `userHeader` consistently with your clients. Image repositories, tag,
controller/load-balancer pull policy and resources are values. Injected sidecars
retain the controller's current resource defaults and Kubernetes' tag-based pull
policy. Restart application pods after changing their ConfigMap; environment
variables are captured at process startup. Existing injected Deployments must be
recreated from their original manifests when changing the sidecar image.

The chart generates a one-year self-signed webhook certificate and reuses its
Secret during upgrades. Plan certificate renewal before expiry; automated rotation
is not included. Helm requires permission to manage the cluster-scoped
MutatingWebhookConfiguration, whose selector restricts injection to this namespace.

To remove the installation, remove your application first, then the release:

```sh
kubectl -n chat delete deployment YOUR_APPLICATION
helm uninstall chat --namespace chat --wait
```

Helm removes the discovery Service, webhook and TLS Secret. It leaves the namespace,
application resources and any existing ServiceAccount you supplied under your control.

Run `WS_E2E_INSTALL=helm WS_OPERATOR_USER_HEADER=X-App-User make system-test`
to build local images, install into a disposable kind namespace, upgrade without
changing TLS trust, run the full message/recovery suite, and verify uninstall.
This command owns and deletes its cluster; it does not use your active kubeconfig.
