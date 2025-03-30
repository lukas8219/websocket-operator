# WebSocket Operator Controller

This controller is a Kubernetes admission webhook that automatically injects a WebSocket proxy sidecar into deployments with the label `lukas8219/ws-operator/enabled: "true"`.

## Prerequisites

- Docker
- kubectl
- A Kubernetes cluster
- Access to a Docker registry (optional for pushing images)

## Building the Docker Image

To build the Docker image, run:

```bash
# From the controller directory
chmod +x build.sh
./build.sh
```

To build and push the image to a registry:

```bash
# From the controller directory
PUSH=true REGISTRY=your-registry REPOSITORY=your-repo ./build.sh
```

## Deploying to Kubernetes

To deploy the controller to Kubernetes, run:

```bash
# From the controller directory
chmod +x deploy.sh
./deploy.sh
```

You can customize the deployment by setting the following environment variables:

```bash
REGISTRY=your-registry REPOSITORY=your-repo TAG=your-tag ./deploy.sh
```

## TLS Certificate for the Webhook

The webhook requires TLS certificates to operate securely. The deployment script assumes you have a file named `ca-bundle.pem` in the controller directory. For a production environment, you should:

1. Generate a proper TLS certificate and key for the webhook service
2. Create a Secret containing these certificates
3. Mount the Secret in the deployment
4. Update the MutatingWebhookConfiguration with the proper CA bundle

## Using the Operator

To have the operator inject the WebSocket proxy sidecar into your deployments, add the following label to your deployment:

```yaml
metadata:
  labels:
    lukas8219/ws-operator/enabled: "true"
```

The controller will automatically inject the WebSocket proxy sidecar container (`lukas8219/websocket-proxy:latest`) into these deployments.

## Troubleshooting

If the webhook isn't working as expected, check:

1. The controller pod logs: `kubectl logs -n websocket-system -l app=websocket-operator-controller`
2. Ensure the label is correctly applied to your deployments
3. Verify the webhook configuration is correctly applied: `kubectl get mutatingwebhookconfigurations` 