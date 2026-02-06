# Database Sync Deployment Notes

## PostgreSQL Secret Configuration

The database sync feature requires access to the PostgreSQL credentials from the `partnerlabs-datastore-pguser-labrat` secret.

### Option 1: Copy Secret to Operator Namespace (Recommended)

Since Kubernetes doesn't support cross-namespace secret references in environment variables, you need to copy the secret to the operator's namespace:

```bash
# Copy the PostgreSQL secret from partnerlabs-wg-cluster to the operator namespace
kubectl get secret partnerlabs-datastore-pguser-labrat \
  -n partnerlabs-wg-cluster \
  -o yaml | \
  sed 's/namespace: partnerlabs-wg-cluster/namespace: opl-cluster-operator-system/' | \
  kubectl apply -f -
```

### Option 2: Use External Secrets Operator (Alternative)

If you have External Secrets Operator installed, you can create an ExternalSecret resource:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: partnerlabs-datastore-pguser-labrat
  namespace: opl-cluster-operator-system
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: cluster-secret-store
    kind: ClusterSecretStore
  target:
    name: partnerlabs-datastore-pguser-labrat
  dataFrom:
  - find:
      name:
        regexp: ^partnerlabs-datastore-pguser-labrat$
```

### Option 3: Disable Database Sync

If you don't want database sync functionality, comment out the `DB_HOST` environment variable in [config/manager/manager.yaml](manager.yaml):

```yaml
# Database configuration (optional - comment out to disable database sync)
# - name: DB_HOST
#   valueFrom:
#     secretKeyRef:
#       name: partnerlabs-datastore-pguser-labrat
#       key: host
```

When `DB_HOST` is not set, the operator will skip database sync initialization and only handle manual ClusterRequest creation.

## Deployment Steps

1. **Apply database migration** (if not already done):
   ```bash
   # Get database credentials
   export PGHOST=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n partnerlabs-wg-cluster -o jsonpath='{.data.host}' | base64 -d)
   export PGPORT=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n partnerlabs-wg-cluster -o jsonpath='{.data.port}' | base64 -d)
   export PGDATABASE=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n partnerlabs-wg-cluster -o jsonpath='{.data.dbname}' | base64 -d)
   export PGUSER=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n partnerlabs-wg-cluster -o jsonpath='{.data.user}' | base64 -d)
   export PGPASSWORD=$(kubectl get secret partnerlabs-datastore-pguser-labrat -n partnerlabs-wg-cluster -o jsonpath='{.data.password}' | base64 -d)

   # Apply migration
   psql -f db/migrations/001_add_k8s_sync_columns.sql
   ```

2. **Copy PostgreSQL secret** (see Option 1 above)

3. **Deploy operator**:
   ```bash
   make deploy IMG=quay.io/yoza/opl-cluster-operator:v0.2.3
   ```

4. **Verify database sync is running**:
   ```bash
   kubectl logs -n opl-cluster-operator-system deployment/opl-cluster-operator-controller-manager | grep "database"
   ```

   You should see:
   ```
   INFO    Initializing database client
   INFO    Database client initialized successfully
   INFO    Database sync controller initialized successfully
   INFO    Starting database sync polling loop
   ```

## Troubleshooting

### Secret Not Found Error

If you see:
```
Error: secret "partnerlabs-datastore-pguser-labrat" not found
```

Make sure you copied the secret to the operator namespace (see Option 1 above).

### Database Connection Errors

Check the secret values:
```bash
kubectl get secret partnerlabs-datastore-pguser-labrat -n opl-cluster-operator-system -o yaml
```

Verify the operator can reach the database:
```bash
# Port-forward to test connectivity
kubectl port-forward -n opl-cluster-operator-system deployment/opl-cluster-operator-controller-manager 5432:5432
psql -h localhost -U <username> -d <database>
```

### Database Sync Not Running

Check if `DB_HOST` is set:
```bash
kubectl get deployment -n opl-cluster-operator-system opl-cluster-operator-controller-manager -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="DB_HOST")]}'
```

If empty, the database sync feature is disabled.
