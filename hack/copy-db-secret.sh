#!/bin/bash

# Script to copy PostgreSQL secret to operator namespace
# This is needed because Kubernetes doesn't support cross-namespace secret references

set -e

SOURCE_NAMESPACE="partnerlabs-wg-cluster"
SOURCE_SECRET="partnerlabs-datastore-pguser-labrat"
TARGET_NAMESPACE="opl-cluster-operator-system"

echo "Copying PostgreSQL secret from $SOURCE_NAMESPACE to $TARGET_NAMESPACE..."

# Check if source secret exists
if ! kubectl get secret "$SOURCE_SECRET" -n "$SOURCE_NAMESPACE" &>/dev/null; then
    echo "Error: Secret $SOURCE_SECRET not found in namespace $SOURCE_NAMESPACE"
    exit 1
fi

# Check if target namespace exists
if ! kubectl get namespace "$TARGET_NAMESPACE" &>/dev/null; then
    echo "Warning: Namespace $TARGET_NAMESPACE does not exist. Creating it..."
    kubectl create namespace "$TARGET_NAMESPACE"
fi

# Copy the secret
kubectl get secret "$SOURCE_SECRET" -n "$SOURCE_NAMESPACE" -o yaml | \
    sed "s/namespace: $SOURCE_NAMESPACE/namespace: $TARGET_NAMESPACE/" | \
    kubectl apply -f -

echo "✓ Secret copied successfully!"
echo ""
echo "Verify the secret:"
echo "  kubectl get secret $SOURCE_SECRET -n $TARGET_NAMESPACE"
echo ""
echo "View secret data (decoded):"
echo "  kubectl get secret $SOURCE_SECRET -n $TARGET_NAMESPACE -o jsonpath='{.data.host}' | base64 -d && echo"
