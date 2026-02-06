#!/bin/bash

# Script to test PostgreSQL connection using the same credentials as the operator

set -e

echo "Testing PostgreSQL connection from opl-cluster-operator-system namespace..."
echo ""

# Extract credentials from secret
echo "Extracting database credentials from secret..."
DB_HOST=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n opl-cluster-operator-system -o jsonpath='{.data.host}' | base64 -d)
DB_PORT=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n opl-cluster-operator-system -o jsonpath='{.data.port}' | base64 -d)
DB_USER=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n opl-cluster-operator-system -o jsonpath='{.data.user}' | base64 -d)
DB_PASSWORD=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n opl-cluster-operator-system -o jsonpath='{.data.password}' | base64 -d)
DB_NAME=$(kubectl get configmap opl-cluster-operator-database-sync-config -n opl-cluster-operator-system -o jsonpath='{.data.DB_NAME}' 2>/dev/null || echo "openshift_partner_labs_app_staging")

echo "Connection details:"
echo "  Host: $DB_HOST"
echo "  Port: $DB_PORT"
echo "  User: $DB_USER"
echo "  Database: $DB_NAME"
echo ""

# Create a test pod
echo "Creating test pod in opl-cluster-operator-system namespace..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: pg-connection-test
  namespace: opl-cluster-operator-system
spec:
  restartPolicy: Never
  containers:
  - name: postgres
    image: postgres:15
    env:
    - name: PGHOST
      value: "$DB_HOST"
    - name: PGPORT
      value: "$DB_PORT"
    - name: PGUSER
      value: "$DB_USER"
    - name: PGPASSWORD
      value: "$DB_PASSWORD"
    - name: PGDATABASE
      value: "$DB_NAME"
    command:
    - /bin/bash
    - -c
    - |
      echo "Testing connection to PostgreSQL..."
      echo "Host: \$PGHOST:\$PGPORT"
      echo "Database: \$PGDATABASE"
      echo "User: \$PGUSER"
      echo ""

      # Test basic connectivity
      echo "1. Testing network connectivity..."
      if nc -zv \$PGHOST \$PGPORT 2>&1; then
        echo "✓ Network connection successful"
      else
        echo "✗ Network connection failed"
        exit 1
      fi

      echo ""
      echo "2. Testing PostgreSQL authentication and connection..."
      if psql -c "SELECT version();" 2>&1; then
        echo "✓ PostgreSQL connection successful"
      else
        echo "✗ PostgreSQL connection failed"
        exit 1
      fi

      echo ""
      echo "3. Listing tables in database..."
      psql -c "\dt" 2>&1 || true

      echo ""
      echo "4. Testing access to labs table..."
      psql -c "SELECT COUNT(*) FROM labs;" 2>&1 || echo "✗ Cannot access labs table"

      echo ""
      echo "5. Checking for k8s_synced column..."
      psql -c "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'labs' AND column_name IN ('k8s_synced', 'k8s_namespace', 'k8s_created_at');" 2>&1 || true

      echo ""
      echo "Connection test completed!"
EOF

echo ""
echo "Waiting for test pod to complete..."
kubectl wait --for=condition=Ready pod/pg-connection-test -n opl-cluster-operator-system --timeout=30s 2>/dev/null || true

echo ""
echo "Test results:"
echo "============================================"
kubectl logs pg-connection-test -n opl-cluster-operator-system --tail=100

echo ""
echo "============================================"
echo ""
echo "Cleaning up test pod..."
kubectl delete pod pg-connection-test -n opl-cluster-operator-system --wait=false

echo ""
echo "To run an interactive session, use:"
echo "  kubectl run pg-test --rm -it --image=postgres:15 -n opl-cluster-operator-system --env=\"PGHOST=$DB_HOST\" --env=\"PGPORT=$DB_PORT\" --env=\"PGUSER=$DB_USER\" --env=\"PGPASSWORD=$DB_PASSWORD\" --env=\"PGDATABASE=$DB_NAME\" -- bash"
