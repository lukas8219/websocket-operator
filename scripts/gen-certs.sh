#!/usr/bin/env bash
# Generate development webhook certificates and render their Kubernetes manifests.
set -euo pipefail
umask 077
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cert_dir="${CERT_DIR:-$repo_root/deployments/certs}"
manifest_dir="${MANIFEST_DIR:-$repo_root/deployments/k8s}"
service_name=websocket-operator-webhook
namespace=websocket-system
mkdir -p "$cert_dir" "$manifest_dir"

openssl genrsa -out "$cert_dir/ca.key" 2048
openssl req -new -x509 -days 365 -key "$cert_dir/ca.key" \
  -subj '/CN=Webhook CA' -out "$cert_dir/ca.crt"
openssl genrsa -out "$cert_dir/server.key" 2048
openssl req -new -key "$cert_dir/server.key" \
  -subj "/CN=$service_name.$namespace.svc" -out "$cert_dir/server.csr"
cat > "$cert_dir/server.ext" <<EXT
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:$service_name,DNS:$service_name.$namespace,DNS:$service_name.$namespace.svc,DNS:$service_name.$namespace.svc.cluster.local
EXT
openssl x509 -req -in "$cert_dir/server.csr" -CA "$cert_dir/ca.crt" \
  -CAkey "$cert_dir/ca.key" -CAcreateserial -out "$cert_dir/server.crt" \
  -days 365 -extfile "$cert_dir/server.ext"

ca_bundle=$(base64 < "$cert_dir/ca.crt" | tr -d '\n')
tls_cert=$(base64 < "$cert_dir/server.crt" | tr -d '\n')
tls_key=$(base64 < "$cert_dir/server.key" | tr -d '\n')
sed "s|\${TLS_CERT}|$tls_cert|g;s|\${TLS_KEY}|$tls_key|g" \
  "$repo_root/deployments/templates/tls-secrets.yaml" > "$manifest_dir/tls-secrets-generated.yaml"
sed "s|\${CA_BUNDLE}|$ca_bundle|g" \
  "$repo_root/deployments/templates/webhook.yaml" > "$manifest_dir/webhook-generated.yaml"
printf '%s\n' "$ca_bundle" > "$cert_dir/ca-bundle.base64"
printf 'Certificates: %s\nManifests: %s\n' "$cert_dir" "$manifest_dir"
