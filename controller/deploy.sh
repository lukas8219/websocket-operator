#!/bin/bash

# Exit on error
set -e

# Set variables
REGISTRY=${REGISTRY:-"docker.io"}
REPOSITORY=${REPOSITORY:-"lukas8219"}
IMAGE_NAME=${IMAGE_NAME:-"websocket-operator-controller"}
TAG=${TAG:-"latest"}

# Full image name
FULL_IMAGE_NAME="${REGISTRY}/${REPOSITORY}/${IMAGE_NAME}:${TAG}"

# Generate certificates if they don't exist
if [ ! -d "certs" ] || [ ! -f "ca-bundle.base64" ]; then
  echo "Generating TLS certificates..."
  chmod +x gen-certs.sh
  ./gen-certs.sh
else
  echo "Using existing TLS certificates"
fi

# Create the namespace
kubectl apply -f k8s/namespace.yaml

# Update the deployment manifest with the correct image
sed "s|image: websocket-operator-controller:latest|image: ${FULL_IMAGE_NAME}|g" k8s/deployment.yaml > k8s/deployment-generated.yaml

# Apply the TLS Secret
kubectl apply -f k8s/tls-secrets-generated.yaml

# Deploy the resources
kubectl apply -f k8s/deployment-generated.yaml
kubectl apply -f k8s/service.yaml

# Apply the webhook configuration
kubectl apply -f k8s/webhook-generated.yaml

echo "Deployment complete!"
echo "To test the webhook, create a deployment with the label 'ws.operator/enabled: \"true\"'" 