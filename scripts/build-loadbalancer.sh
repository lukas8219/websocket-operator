#!/bin/bash

# Exit on error
set -e

# Set variables
REGISTRY=${REGISTRY:-"docker.io"}
REPOSITORY=${REPOSITORY:-"lukas8219"}
IMAGE_NAME=${IMAGE_NAME:-"websocket-operator-loadbalancer"}
TAG=${TAG:-"latest"}

# Full image name
FULL_IMAGE_NAME="${REGISTRY}/${REPOSITORY}/${IMAGE_NAME}:${TAG}"

echo "Building image: ${FULL_IMAGE_NAME}"

# Build the Docker image
docker build -f cmd/loadbalancer/Dockerfile -t ${FULL_IMAGE_NAME} ${BUILD_ARGS} .

# Push the Docker image
if [ "${PUSH:-false}" == "true" ]; then
  echo "Pushing image: ${FULL_IMAGE_NAME}"
  docker push ${FULL_IMAGE_NAME}
fi

echo "Done!" 