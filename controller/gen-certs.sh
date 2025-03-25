#!/bin/bash

# Exit on error
set -e

# Set variables
SERVICE_NAME=websocket-operator-webhook
NAMESPACE=websocket-system
SECRET_NAME=webhook-tls
CERT_DIR=certs

# Create certificate directory
mkdir -p ${CERT_DIR}
cd ${CERT_DIR}

# Generate a CA key and certificate
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 365 -key ca.key -subj "/CN=Webhook CA" -out ca.crt

# Create a server key and certificate signing request (CSR)
openssl genrsa -out server.key 2048
openssl req -new -key server.key -subj "/CN=${SERVICE_NAME}.${NAMESPACE}.svc" -out server.csr

# Create a certificate config file for the server certificate
cat > server.ext << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${SERVICE_NAME}
DNS.2 = ${SERVICE_NAME}.${NAMESPACE}
DNS.3 = ${SERVICE_NAME}.${NAMESPACE}.svc
DNS.4 = ${SERVICE_NAME}.${NAMESPACE}.svc.cluster.local
EOF

# Create the server certificate signed by the CA
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 365 -extfile server.ext

# Base64 encode the certificates and keys
CA_BUNDLE=$(cat ca.crt | base64 | tr -d '\n')
TLS_CERT=$(cat server.crt | base64 | tr -d '\n')
TLS_KEY=$(cat server.key | base64 | tr -d '\n')

cd ..

# Update the Secret and MutatingWebhookConfiguration with the certificates
sed "s|\${TLS_CERT}|${TLS_CERT}|g;s|\${TLS_KEY}|${TLS_KEY}|g" \
    templates/tls-secrets.yaml > k8s/tls-secrets-generated.yaml

sed "s|\${CA_BUNDLE}|${CA_BUNDLE}|g" \
    templates/webhook.yaml > k8s/webhook-generated.yaml

# Output the CA bundle for reference
echo "CA_BUNDLE: ${CA_BUNDLE}"
echo "Certificates generated successfully. Stored in ${CERT_DIR}/"

# Save CA bundle to a file for later use
echo ${CA_BUNDLE} > ca-bundle.base64 