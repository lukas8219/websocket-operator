#!/usr/bin/env bash
# Sourced only by test-system.sh inside its owned cluster.
helm_values=(--namespace "$namespace" --set-string "images.tag=$image_tag"
  --set-string images.controller=websocket-operator-controller
  --set-string images.loadbalancer=websocket-operator-loadbalancer
  --set-string images.sidecar=websocket-operator-sidecar
  --set-string "userHeader=$WS_E2E_USER_HEADER")
helm install chat charts/websocket-operator "${helm_values[@]}" --create-namespace --wait --timeout 3m
before=$(kubectl get secret chat-ws-tls -o jsonpath='{.data.tls\.crt}')
helm upgrade chat charts/websocket-operator "${helm_values[@]}" --wait --timeout 3m
after=$(kubectl get secret chat-ws-tls -o jsonpath='{.data.tls\.crt}')
[[ -n "$before" && "$before" == "$after" ]] || { echo 'Upgrade replaced webhook trust' >&2; exit 1; }
kubectl get mutatingwebhookconfiguration "chat-ws-$namespace" -o json |
  jq -e --arg namespace "$namespace" '.webhooks[0].namespaceSelector.matchLabels["kubernetes.io/metadata.name"] == $namespace'
