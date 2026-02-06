# Database Synchronization Controller - Design Document

## Executive Summary

This document describes the design and implementation of a **bidirectional database synchronization controller** that integrates the OPL Cluster Operator with a PostgreSQL database. The controller enables automatic creation of ClusterRequest custom resources from database records and synchronizes status updates back to the database.

**Key Features:**
- **DB → Kubernetes Sync**: Polls PostgreSQL for new lab requests and creates ClusterRequest CRs
- **Kubernetes → DB Sync**: Updates database when ClusterRequest status changes
- **Idempotency**: Prevents duplicate CR creation through multiple safeguards
- **Fault Tolerance**: Handles connection failures, invalid data, and conflicts gracefully
- **Configurable**: Polling intervals, batch sizes, and connection settings via ConfigMap/Secret

---

## Motivation & Use Case

### Current Workflow (Manual)

```
┌─────────────┐      Manual      ┌─────────────┐
│  PostgreSQL │   ────────────►  │ Kubernetes  │
│   Database  │    Admin/Script  │ ClusterReq  │
│   (labs)    │                  │     CRs     │
└─────────────┘                  └─────────────┘
```

**Problems:**
- Manual intervention required to create CRs from database records
- No automatic status synchronization back to database
- Database and Kubernetes state can drift
- Buffalo app has no visibility into provisioning status without polling Kubernetes

### Proposed Workflow (Automated)

```
┌─────────────┐    Automated     ┌─────────────┐
│  PostgreSQL │   ◄──────────►   │ Kubernetes  │
│   Database  │   DB Sync Ctrl   │ ClusterReq  │
│   (labs)    │                  │     CRs     │
└─────────────┘                  └─────────────┘
```

**Benefits:**
- ✅ Zero manual intervention - labs auto-provision when approved in database
- ✅ Real-time status updates in database (git-committed, provisioning, complete, failed)
- ✅ Single source of truth: database for requests, Kubernetes for lifecycle
- ✅ Buffalo app can display accurate provisioning status from database
- ✅ Audit trail in both systems

---

## System Architecture

### High-Level Overview

```
┌───────────────────────────────────────────────────────────────────┐
│                    Buffalo Web Application                        │
│                  (User Interface + Admin Portal)                  │
│                                                                    │
│  User requests lab → Admin approves → labs table updated          │
└──────────────────────────┬────────────────────────────────────────┘
                           │
                           │ PostgreSQL Database
                           │
                           ▼
                  ┌────────────────────┐
                  │   PostgreSQL DB    │
                  │   ┌──────────────┐ │
                  │   │  labs table  │ │
                  │   │  - id        │ │
                  │   │  - cluster_  │ │
                  │   │    name      │ │
                  │   │  - state     │ │
                  │   │  - k8s_      │ │
                  │   │    synced    │ │
                  │   └──────────────┘ │
                  └──────────┬─────────┘
                             │
                             │ Polling (every 30s)
                             │ SELECT WHERE k8s_synced = false
                             │
                             ▼
              ┌──────────────────────────────┐
              │  Database Sync Reconciler    │
              │  (Dual Mode Controller)      │
              │                              │
              │  Mode 1: Polling Loop        │
              │    • Query unsynced labs     │
              │    • Create ClusterRequest   │
              │    • Mark as synced          │
              │                              │
              │  Mode 2: Watch ClusterReq    │
              │    • Detect status changes   │
              │    • Update database         │
              └──────────┬───────────────────┘
                         │
              ┌──────────┴───────────┐
              │                      │
              ▼                      ▼
     ┌────────────────┐    ┌────────────────────┐
     │  Kubernetes    │    │   PostgreSQL DB    │
     │  API Server    │    │   UPDATE labs      │
     │                │    │   SET state = ...  │
     │  ClusterReq CR │    └────────────────────┘
     └────────┬───────┘
              │
              ▼
     ┌────────────────────────┐
     │ ClusterRequest         │
     │ Reconciler             │
     │ (Existing Controller)  │
     │                        │
     │ • Git commits          │
     │ • ArgoCD sync          │
     │ • Hive monitoring      │
     └────────────────────────┘
```

### Component Interaction Flow

**Flow 1: New Lab Request (DB → K8s)**

```
1. User submits lab request in Buffalo app
   └─► INSERT INTO labs (cluster_name='test-01', state='approved', k8s_synced=false)

2. Database Sync Controller (polling every 30s)
   └─► SELECT * FROM labs WHERE k8s_synced = false
   └─► Found lab ID 123

3. Idempotency Checks:
   a) Check if CR exists by name: kubectl get cr test-01
   b) Check if CR exists by label: kubectl get cr -l lab-id=123
   c) Validate data (DNS compliance, enums, required fields)

4. Fetch company name:
   └─► SELECT company_name FROM companies WHERE id = lab.company_id

5. Create ClusterRequest CR:
   └─► apiVersion: opl.openshiftpartnerlabs.com/v1alpha1
       kind: ClusterRequest
       metadata:
         name: test-01
         labels:
           opl.openshiftpartnerlabs.com/lab-id: "123"
           opl.openshiftpartnerlabs.com/source: "database"
       spec:
         clusterName: test-01
         environment: development
         labId: 123
         ...

6. Mark as synced:
   └─► UPDATE labs SET k8s_synced = true, k8s_namespace = 'default' WHERE id = 123

7. ClusterRequest Reconciler takes over:
   └─► State: pending → approved → git-committed → provisioning → complete
```

**Flow 2: Status Update (K8s → DB)**

```
1. ClusterRequest Reconciler updates status:
   └─► status.state = "git-committed"
   └─► status.gitCommitSHA = "abc123"

2. Database Sync Controller (watches ClusterRequest):
   └─► Reconcile() triggered by status change

3. Check if CR is database-sourced:
   └─► spec.labId = 123 (non-zero means from database)

4. Update database:
   └─► UPDATE labs
       SET state = 'git-committed',
           updated_at = NOW()
       WHERE id = 123

5. Buffalo app queries database:
   └─► SELECT state FROM labs WHERE id = 123
   └─► Returns: "git-committed"
   └─► UI displays: "Cluster configuration committed to Git ✓"
```

---

## Database Schema

### Existing `labs` Table

```sql
CREATE TABLE labs (
    id              integer      PRIMARY KEY,
    cluster_id      char(36)     NOT NULL,
    generated_name  varchar(32)  NOT NULL,
    state           varchar(12)  NOT NULL,
    cluster_name    varchar(32)  NOT NULL,
    openshift_version varchar(16) NOT NULL,
    cluster_size    varchar(7)   NOT NULL,
    company_id      integer      NULL,
    request_type    varchar(12)  NOT NULL,
    partner         smallint     NOT NULL,
    sponsor         varchar(64)  NOT NULL,
    cloud_provider  varchar(8)   NOT NULL,
    primary_first   varchar(32)  NOT NULL,
    primary_last    varchar(32)  NOT NULL,
    primary_email   varchar(64)  NOT NULL,
    secondary_first varchar(32)  NOT NULL,
    secondary_last  varchar(32)  NOT NULL,
    secondary_email varchar(64)  NOT NULL,
    region          varchar(5)   NOT NULL,
    always_on       smallint     NOT NULL,
    project_name    varchar(32)  NOT NULL,
    lease_time      varchar(2)   NOT NULL,
    description     text         NOT NULL,
    notes           text         NOT NULL,
    start_date      timestamp    NOT NULL,
    end_date        timestamp    NOT NULL,
    hold            smallint     NOT NULL,
    created_at      timestamp    NOT NULL,
    updated_at      timestamp    NOT NULL,
    CONSTRAINT labs_pk UNIQUE (generated_name),
    CONSTRAINT labs_companies_id_fk
        FOREIGN KEY (company_id) REFERENCES companies (id)
);
```

### Required Migration

**File: `db/migrations/001_add_k8s_sync_columns.sql`**

```sql
-- Add columns to track Kubernetes synchronization
ALTER TABLE labs ADD COLUMN IF NOT EXISTS k8s_synced BOOLEAN DEFAULT false;
ALTER TABLE labs ADD COLUMN IF NOT EXISTS k8s_namespace VARCHAR(63);
ALTER TABLE labs ADD COLUMN IF NOT EXISTS k8s_created_at TIMESTAMP;

-- Create index for performance (query on k8s_synced is frequent)
CREATE INDEX IF NOT EXISTS idx_labs_k8s_synced ON labs(k8s_synced) WHERE k8s_synced = false;

-- Add comments for documentation
COMMENT ON COLUMN labs.k8s_synced IS 'True if ClusterRequest CR has been created in Kubernetes';
COMMENT ON COLUMN labs.k8s_namespace IS 'Kubernetes namespace where ClusterRequest CR exists';
COMMENT ON COLUMN labs.k8s_created_at IS 'Timestamp when ClusterRequest CR was created';
```

**Column Descriptions:**

| Column | Type | Nullable | Purpose |
|--------|------|----------|---------|
| `k8s_synced` | boolean | NOT NULL | Idempotency flag - prevents duplicate CR creation |
| `k8s_namespace` | varchar(63) | NULL | Tracks which K8s namespace contains the CR |
| `k8s_created_at` | timestamp | NULL | Audit trail - when CR was created |

**Index Rationale:**
- Partial index on `k8s_synced = false` optimizes the frequent query for unsynced labs
- Only indexes rows that need syncing (efficient for large tables with mostly synced records)

### State Mapping

**Database → Kubernetes (Initial Creation)**

| labs.state | ClusterRequest.status.state | Action |
|------------|----------------------------|---------|
| `pending` | `pending` | Create CR, wait for approval |
| `approved` | `approved` | Create CR, start provisioning |

**Kubernetes → Database (Status Updates)**

| ClusterRequest.status.state | labs.state Update | Additional Fields |
|----------------------------|-------------------|-------------------|
| `git-committed` | `git-committed` | - |
| `provisioning` | `provisioning` | - |
| `complete` | `complete` | - |
| `failed` | `failed` | Store error in notes field |

**State Not Synced Back:**
- `pending`, `approved` - Database is source of truth for these initial states
- Only sync lifecycle states that Kubernetes manages

---

## Field Mapping

### labs Table → ClusterRequest Spec

| Source Field (labs) | Target Field (ClusterRequest) | Transformation |
|---------------------|------------------------------|----------------|
| `cluster_name` | `spec.clusterName` | Direct mapping (must be DNS-compliant) |
| `request_type` | `spec.environment` | Enum mapping: <br/>• "demo", "trial" → "development"<br/>• "production" → "production"<br/>• default → "staging" |
| `openshift_version` | `spec.openshiftVersion` | Direct mapping |
| `cluster_size` | `spec.clusterSize` | Direct mapping (small/medium/large) |
| `region` | `spec.region` | Direct mapping (AWS region) |
| `primary_email` | `spec.requestedBy` | Direct mapping |
| `company_id` | `spec.companyName` | **JOIN**: SELECT company_name FROM companies WHERE id = company_id |
| `id` | `spec.labId` | Direct mapping (critical for bidirectional sync) |
| N/A | `spec.baseDomain` | Default: "openshiftpartnerlabs.com" (configurable) |

### Validation Rules

Before creating ClusterRequest, validate:

```go
func validateLab(lab database.Lab) error {
    // DNS compliance for cluster name
    if !isDNSCompliant(lab.ClusterName) {
        return fmt.Errorf("cluster_name not DNS-compliant: %s", lab.ClusterName)
    }

    // Valid cluster size
    validSizes := map[string]bool{"small": true, "medium": true, "large": true}
    if !validSizes[lab.ClusterSize] {
        return fmt.Errorf("invalid cluster_size: %s (must be small, medium, or large)", lab.ClusterSize)
    }

    // Valid region
    if lab.Region == "" {
        return fmt.Errorf("region cannot be empty")
    }

    // Valid email
    if !isValidEmail(lab.PrimaryEmail) {
        return fmt.Errorf("invalid primary_email: %s", lab.PrimaryEmail)
    }

    // Check hold flag
    if lab.Hold != 0 {
        return fmt.Errorf("lab is on hold, skipping sync")
    }

    return nil
}
```

---

## Implementation Details

### Package Structure

```
internal/
├── controller/
│   ├── clusterrequest_controller.go       # Existing controller
│   ├── dbsync_controller.go               # NEW: Database sync controller
│   └── dbsync_controller_test.go          # NEW: Controller tests
└── database/
    ├── client.go                          # NEW: PostgreSQL client (pgx)
    ├── config.go                          # NEW: Configuration structs
    ├── models.go                          # NEW: Lab, Company models
    ├── queries.go                         # NEW: Query methods
    ├── mapper.go                          # NEW: Lab → ClusterRequest mapping
    ├── client_test.go                     # NEW: Unit tests (pgxmock)
    └── integration_test.go                # NEW: Integration tests (testcontainers)
```

### Database Client (`internal/database/client.go`)

**Design Decisions:**

1. **Connection Pooling**: Use `pgx/v5` connection pool for efficiency
   - Min connections: 2 (maintain warm connections)
   - Max connections: 10 (prevent database overload)
   - Connection max lifetime: 1 hour (rotate connections)
   - Connection max idle time: 10 minutes (release idle connections)

2. **Context-Aware**: All methods accept `context.Context` for:
   - Cancellation support
   - Request tracing
   - Timeout enforcement

3. **Transaction Support**: Critical operations use PostgreSQL transactions
   - Prevents race conditions with multiple operator replicas
   - Uses `SELECT ... FOR UPDATE SKIP LOCKED` for row-level locking

**Key Methods:**

```go
package database

import (
    "context"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
    pool   *pgxpool.Pool
    config Config
}

type Config struct {
    Host            string
    Port            int
    Database        string
    Username        string
    Password        string
    SSLMode         string        // disable, require, verify-ca, verify-full
    MaxConns        int
    MinConns        int
    ConnMaxLifetime time.Duration
    ConnMaxIdleTime time.Duration
}

// NewClient creates a database client
func NewClient(cfg Config) (*Client, error) {
    connString := fmt.Sprintf(
        "postgres://%s:%s@%s:%d/%s?sslmode=%s",
        cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode,
    )

    poolConfig, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, fmt.Errorf("invalid database config: %w", err)
    }

    poolConfig.MaxConns = int32(cfg.MaxConns)
    poolConfig.MinConns = int32(cfg.MinConns)
    poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
    poolConfig.MaxConnIdleTime = cfg.ConnMaxIdleTime

    return &Client{config: cfg, pool: nil}, nil
}

// Initialize establishes the connection pool
func (c *Client) Initialize(ctx context.Context) error {
    pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
    if err != nil {
        return fmt.Errorf("failed to create connection pool: %w", err)
    }

    // Test connection
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return fmt.Errorf("failed to ping database: %w", err)
    }

    c.pool = pool
    return nil
}

// Close closes the connection pool
func (c *Client) Close() {
    if c.pool != nil {
        c.pool.Close()
    }
}

// Ping checks database connectivity
func (c *Client) Ping(ctx context.Context) error {
    if c.pool == nil {
        return fmt.Errorf("connection pool not initialized")
    }
    return c.pool.Ping(ctx)
}
```

### Query Methods (`internal/database/queries.go`)

**Critical Query: Get Unsynced Labs**

```go
// GetUnsyncedLabs retrieves labs ready for Kubernetes sync
func (c *Client) GetUnsyncedLabs(ctx context.Context, batchSize int) ([]Lab, error) {
    query := `
        SELECT
            id, cluster_id, generated_name, state, cluster_name,
            openshift_version, cluster_size, company_id, request_type,
            primary_email, region, created_at, updated_at, hold
        FROM labs
        WHERE k8s_synced = false
          AND state IN ('pending', 'approved')
          AND hold = 0
        ORDER BY created_at ASC
        FOR UPDATE SKIP LOCKED  -- Multi-replica safety
        LIMIT $1
    `

    rows, err := c.pool.Query(ctx, query, batchSize)
    if err != nil {
        return nil, fmt.Errorf("failed to query unsynced labs: %w", err)
    }
    defer rows.Close()

    var labs []Lab
    for rows.Next() {
        var lab Lab
        err := rows.Scan(
            &lab.ID, &lab.ClusterID, &lab.GeneratedName, &lab.State,
            &lab.ClusterName, &lab.OpenshiftVersion, &lab.ClusterSize,
            &lab.CompanyID, &lab.RequestType, &lab.PrimaryEmail,
            &lab.Region, &lab.CreatedAt, &lab.UpdatedAt, &lab.Hold,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to scan lab row: %w", err)
        }
        labs = append(labs, lab)
    }

    return labs, rows.Err()
}
```

**FOR UPDATE SKIP LOCKED Explained:**
- `FOR UPDATE`: Locks selected rows for update (prevents concurrent modification)
- `SKIP LOCKED`: If another replica already locked a row, skip it (don't block)
- **Result**: Multiple operator replicas can run simultaneously without conflicts
- Each replica processes different subset of unsynced labs

**Mark Lab as Synced (Transactional)**

```go
// MarkLabAsSynced atomically updates k8s_synced flag
func (c *Client) MarkLabAsSynced(ctx context.Context, labID int, namespace string) error {
    tx, err := c.pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback(ctx) // Rollback if not committed

    query := `
        UPDATE labs
        SET k8s_synced = true,
            k8s_namespace = $1,
            k8s_created_at = NOW(),
            updated_at = NOW()
        WHERE id = $2
    `

    result, err := tx.Exec(ctx, query, namespace, labID)
    if err != nil {
        return fmt.Errorf("failed to mark lab as synced: %w", err)
    }

    if result.RowsAffected() == 0 {
        return fmt.Errorf("lab ID %d not found", labID)
    }

    return tx.Commit(ctx)
}

// UpdateLabK8sStatus updates state and metadata
func (c *Client) UpdateLabK8sStatus(ctx context.Context, labID int, status LabK8sStatus) error {
    query := `
        UPDATE labs
        SET state = $1,
            updated_at = NOW()
        WHERE id = $2
    `

    result, err := c.pool.Exec(ctx, query, status.State, labID)
    if err != nil {
        return fmt.Errorf("failed to update lab status: %w", err)
    }

    if result.RowsAffected() == 0 {
        return fmt.Errorf("lab ID %d not found", labID)
    }

    return nil
}
```

### Database Sync Controller (`internal/controller/dbsync_controller.go`)

**Dual-Mode Operation:**

```go
package controller

import (
    "context"
    "fmt"
    "time"

    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    oplv1alpha1 "github.com/yashoza19/opl-cluster-operator/api/v1alpha1"
    "github.com/yashoza19/opl-cluster-operator/internal/database"
)

// DatabaseSyncReconciler syncs labs from PostgreSQL to ClusterRequests
type DatabaseSyncReconciler struct {
    client.Client
    DBClient *database.Client
    Config   DatabaseSyncConfig
}

type DatabaseSyncConfig struct {
    SyncNamespace   string        // Namespace to create CRs in
    BaseDomain      string        // Default base domain
    PollingInterval time.Duration // How often to poll database
    BatchSize       int           // Max labs per sync batch
}

// Start implements manager.Runnable (Mode 1: Polling Loop)
func (r *DatabaseSyncReconciler) Start(ctx context.Context) error {
    logger := log.FromContext(ctx).WithName("database-sync-poller")
    logger.Info("Starting database sync polling loop",
        "interval", r.Config.PollingInterval,
        "namespace", r.Config.SyncNamespace,
        "batchSize", r.Config.BatchSize)

    ticker := time.NewTicker(r.Config.PollingInterval)
    defer ticker.Stop()

    // Initial sync on startup
    if err := r.syncDatabaseToKubernetes(ctx); err != nil {
        logger.Error(err, "Initial database sync failed")
    }

    for {
        select {
        case <-ctx.Done():
            logger.Info("Stopping database sync polling loop")
            return nil
        case <-ticker.C:
            if err := r.syncDatabaseToKubernetes(ctx); err != nil {
                logger.Error(err, "Database sync failed")
                // Continue polling despite errors
            }
        }
    }
}

// syncDatabaseToKubernetes polls database and creates CRs (DB → K8s)
func (r *DatabaseSyncReconciler) syncDatabaseToKubernetes(ctx context.Context) error {
    logger := log.FromContext(ctx).WithName("db-to-k8s-sync")

    // 1. Fetch unsynced labs from database
    labs, err := r.DBClient.GetUnsyncedLabs(ctx, r.Config.BatchSize)
    if err != nil {
        return fmt.Errorf("failed to fetch unsynced labs: %w", err)
    }

    if len(labs) == 0 {
        logger.V(1).Info("No unsynced labs found")
        return nil
    }

    logger.Info("Found unsynced labs", "count", len(labs))

    // 2. Process each lab
    for _, lab := range labs {
        if err := r.processLab(ctx, lab); err != nil {
            logger.Error(err, "Failed to process lab",
                "labID", lab.ID, "clusterName", lab.ClusterName)
            // Continue processing other labs (don't fail entire batch)
            continue
        }
    }

    return nil
}

// processLab handles a single lab sync
func (r *DatabaseSyncReconciler) processLab(ctx context.Context, lab database.Lab) error {
    logger := log.FromContext(ctx).WithValues("labID", lab.ID, "clusterName", lab.ClusterName)

    // 1. Validate lab data
    if err := database.ValidateLab(lab); err != nil {
        logger.Error(err, "Lab validation failed")
        // Mark as synced with error so we don't retry invalid data
        return r.DBClient.UpdateLabK8sStatus(ctx, lab.ID, database.LabK8sStatus{
            State:        "failed",
            ErrorMessage: ptr.String(err.Error()),
        })
    }

    // 2. Idempotency check: CR exists by name?
    existingCR := &oplv1alpha1.ClusterRequest{}
    err := r.Get(ctx, types.NamespacedName{
        Name:      lab.ClusterName,
        Namespace: r.Config.SyncNamespace,
    }, existingCR)

    if err == nil {
        // CR exists - check if it's the same lab
        if existingCR.Spec.LabID == lab.ID {
            logger.Info("ClusterRequest already exists for this lab, marking as synced")
            return r.DBClient.MarkLabAsSynced(ctx, lab.ID, existingCR.Namespace)
        } else if existingCR.Spec.LabID != 0 {
            // Conflict: different lab with same cluster name
            errMsg := fmt.Sprintf("Name conflict: existing CR has labID=%d, new lab has labID=%d",
                existingCR.Spec.LabID, lab.ID)
            logger.Error(fmt.Errorf(errMsg), "Duplicate cluster name detected")
            return r.DBClient.UpdateLabK8sStatus(ctx, lab.ID, database.LabK8sStatus{
                State:        "failed",
                ErrorMessage: ptr.String(errMsg),
            })
        }
        // If existingCR.Spec.LabID == 0, it was created manually - allow duplicate
    }

    if !errors.IsNotFound(err) {
        return fmt.Errorf("failed to check for existing CR: %w", err)
    }

    // 3. Idempotency check: CR exists with lab-id label?
    crList := &oplv1alpha1.ClusterRequestList{}
    err = r.List(ctx, crList, client.InNamespace(r.Config.SyncNamespace),
        client.MatchingLabels{
            "opl.openshiftpartnerlabs.com/lab-id": fmt.Sprintf("%d", lab.ID),
        })
    if err != nil {
        return fmt.Errorf("failed to list CRs by label: %w", err)
    }

    if len(crList.Items) > 0 {
        logger.Info("ClusterRequest with matching lab-id label found, marking as synced")
        return r.DBClient.MarkLabAsSynced(ctx, lab.ID, crList.Items[0].Namespace)
    }

    // 4. Fetch company name if company_id is set
    var companyName string
    if lab.CompanyID != nil {
        company, err := r.DBClient.GetCompanyByID(ctx, *lab.CompanyID)
        if err != nil {
            logger.Error(err, "Failed to fetch company", "companyID", *lab.CompanyID)
            // Continue with empty company name (non-critical error)
        } else {
            companyName = company.CompanyName
        }
    }

    // 5. Map lab to ClusterRequest
    cr := database.MapLabToClusterRequest(lab, companyName, r.Config.BaseDomain, r.Config.SyncNamespace)

    // 6. Create ClusterRequest
    if err := r.Create(ctx, cr); err != nil {
        return fmt.Errorf("failed to create ClusterRequest: %w", err)
    }

    logger.Info("Created ClusterRequest", "cr", cr.Name, "namespace", cr.Namespace)

    // 7. Mark lab as synced in database
    if err := r.DBClient.MarkLabAsSynced(ctx, lab.ID, cr.Namespace); err != nil {
        logger.Error(err, "Failed to mark lab as synced (CR was created but DB not updated)")
        // Don't return error - CR was created successfully
    }

    return nil
}

// Reconcile handles CR status updates (Mode 2: K8s → DB Sync)
func (r *DatabaseSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // Fetch ClusterRequest
    cr := &oplv1alpha1.ClusterRequest{}
    if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Only process database-sourced CRs (labId != 0)
    if cr.Spec.LabID == 0 {
        logger.V(1).Info("Skipping CR without labId (not from database)")
        return ctrl.Result{}, nil
    }

    // Only sync specific states back to database
    shouldSync := false
    switch cr.Status.State {
    case StateGitCommitted, StateProvisioning, StateComplete, StateFailed:
        shouldSync = true
    }

    if !shouldSync {
        logger.V(1).Info("State not synced back to database", "state", cr.Status.State)
        return ctrl.Result{}, nil
    }

    // Update database with current status
    status := database.LabK8sStatus{
        State:         cr.Status.State,
        GitCommitSHA:  &cr.Status.GitCommitSHA,
        HiveClusterID: &cr.Status.HiveClusterID,
    }

    if cr.Status.State == StateFailed {
        status.ErrorMessage = &cr.Status.ErrorMessage
    }

    if err := r.DBClient.UpdateLabK8sStatus(ctx, cr.Spec.LabID, status); err != nil {
        logger.Error(err, "Failed to sync CR status to database",
            "labID", cr.Spec.LabID, "state", cr.Status.State)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    logger.Info("Synced CR status to database",
        "labID", cr.Spec.LabID, "state", cr.Status.State)

    return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *DatabaseSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&oplv1alpha1.ClusterRequest{}).
        Named("database-sync").
        Complete(r)
}
```

---

## Configuration

### ConfigMap

**File: `config/manager/database-sync-config.yaml`**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: database-sync-config
  namespace: opl-cluster-operator-system
data:
  # Database connection
  DB_HOST: "postgres.database.svc.cluster.local"
  DB_PORT: "5432"
  DB_NAME: "opl_labs"
  DB_SSL_MODE: "require"  # disable, require, verify-ca, verify-full

  # Connection pool settings
  DB_MAX_CONNS: "10"
  DB_MIN_CONNS: "2"

  # Sync configuration
  SYNC_NAMESPACE: "default"
  SYNC_POLLING_INTERVAL: "30s"
  SYNC_BATCH_SIZE: "100"

  # Cluster defaults
  BASE_DOMAIN: "openshiftpartnerlabs.com"
```

### Secret

**File: `config/manager/database-sync-secret.yaml`**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: database-sync-secret
  namespace: opl-cluster-operator-system
type: Opaque
stringData:
  DB_USERNAME: "opl_operator"
  DB_PASSWORD: "changeme"
```

### Deployment Integration

**Modify `config/manager/manager.yaml`:**

```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        image: controller:latest
        env:
        # ... existing Git configuration ...

        # Database configuration
        - name: DB_HOST
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: DB_HOST
        - name: DB_PORT
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: DB_PORT
        - name: DB_NAME
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: DB_NAME
        - name: DB_USERNAME
          valueFrom:
            secretKeyRef:
              name: database-sync-secret
              key: DB_USERNAME
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: database-sync-secret
              key: DB_PASSWORD
        - name: DB_SSL_MODE
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: DB_SSL_MODE
        - name: DB_MAX_CONNS
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: DB_MAX_CONNS
        - name: DB_MIN_CONNS
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: DB_MIN_CONNS
        - name: SYNC_NAMESPACE
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: SYNC_NAMESPACE
        - name: SYNC_POLLING_INTERVAL
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: SYNC_POLLING_INTERVAL
        - name: SYNC_BATCH_SIZE
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: SYNC_BATCH_SIZE
        - name: BASE_DOMAIN
          valueFrom:
            configMapKeyRef:
              name: database-sync-config
              key: BASE_DOMAIN
```

---

## Error Handling & Edge Cases

### 1. Database Connection Failures

**Scenario**: PostgreSQL is unreachable

**Handling**:
- Controller logs error but continues running
- Next polling interval will retry
- Health check endpoint reports unhealthy
- Kubernetes readiness probe can restart pod if needed

**Implementation**:
```go
func (c *Client) InitializeWithRetry(ctx context.Context, maxRetries int) error {
    backoff := 1 * time.Second

    for i := 0; i < maxRetries; i++ {
        if err := c.Initialize(ctx); err == nil {
            return nil
        }

        logger.Error(err, "Database connection failed, retrying",
            "attempt", i+1, "backoff", backoff)

        time.Sleep(backoff)
        backoff *= 2
        if backoff > 30*time.Second {
            backoff = 30 * time.Second
        }
    }

    return fmt.Errorf("failed to connect after %d retries", maxRetries)
}
```

### 2. Duplicate Cluster Names

**Scenario**: Database has lab_id=123 with cluster_name="test", but CR "test" already exists with lab_id=456

**Handling**:
- Detect conflict during idempotency check
- Mark lab_id=123 as failed in database with error message
- Log error for manual resolution
- Don't overwrite existing CR

**Implementation**: See `processLab()` method above

### 3. Invalid Data in Database

**Scenario**: Database has cluster_size="invalid" (not small/medium/large)

**Handling**:
- Validation fails before CR creation
- Mark lab as failed in database with validation error
- Don't retry invalid data (prevents infinite retry loop)

### 4. Multi-Replica Race Conditions

**Scenario**: Two operator replicas running simultaneously

**Handling**:
- Use `SELECT ... FOR UPDATE SKIP LOCKED` in database query
- PostgreSQL ensures only one replica processes each lab
- No duplicates, no conflicts

### 5. Transaction Failures

**Scenario**: CR created successfully but database update fails

**Handling**:
- Log error with details (CR name, lab ID, error)
- Next polling cycle will detect existing CR and mark as synced
- Eventually consistent

---

## Testing Strategy

### Unit Tests

**Database Client Tests (`internal/database/client_test.go`)**

```go
import (
    "github.com/pashagolub/pgxmock/v3"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Database Client", func() {
    var (
        ctx      context.Context
        mockPool pgxmock.PgxPoolIface
        client   *database.Client
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockPool, _ = pgxmock.NewPool()
        client = &database.Client{pool: mockPool}
    })

    Context("GetUnsyncedLabs", func() {
        It("should return unsynced labs", func() {
            rows := pgxmock.NewRows([]string{"id", "cluster_name", "state"}).
                AddRow(1, "test-cluster", "approved").
                AddRow(2, "demo-cluster", "pending")

            mockPool.ExpectQuery("SELECT .* FROM labs WHERE k8s_synced = false").
                WithArgs(100).
                WillReturnRows(rows)

            labs, err := client.GetUnsyncedLabs(ctx, 100)
            Expect(err).NotTo(HaveOccurred())
            Expect(labs).To(HaveLen(2))
            Expect(labs[0].ClusterName).To(Equal("test-cluster"))
        })
    })
})
```

### Integration Tests

**File: `internal/database/integration_test.go`**

```go
// +build integration

import (
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"
)

func TestDatabaseIntegration(t *testing.T) {
    ctx := context.Background()

    // Start PostgreSQL container
    postgres, err := testcontainers.GenericContainer(ctx,
        testcontainers.GenericContainerRequest{
            ContainerRequest: testcontainers.ContainerRequest{
                Image:        "postgres:15",
                ExposedPorts: []string{"5432/tcp"},
                Env: map[string]string{
                    "POSTGRES_DB":       "testdb",
                    "POSTGRES_USER":     "test",
                    "POSTGRES_PASSWORD": "test",
                },
                WaitingFor: wait.ForLog("database system is ready"),
            },
            Started: true,
        })

    defer postgres.Terminate(ctx)

    // Run migration
    // Create test data
    // Test sync
}
```

### E2E Tests

**File: `test/e2e/dbsync_test.go`**

```go
var _ = Describe("Database Sync E2E", func() {
    It("should sync lab from database to Kubernetes", func() {
        By("Inserting lab record in database")
        // INSERT INTO labs (...)

        By("Waiting for sync interval")
        time.Sleep(35 * time.Second)

        By("Verifying ClusterRequest was created")
        cr := &oplv1alpha1.ClusterRequest{}
        err := k8sClient.Get(ctx, types.NamespacedName{
            Name:      "test-cluster",
            Namespace: "default",
        }, cr)
        Expect(err).NotTo(HaveOccurred())
        Expect(cr.Spec.LabID).To(Equal(123))

        By("Verifying lab marked as synced in database")
        // SELECT k8s_synced FROM labs WHERE id = 123
        // Expect: true
    })
})
```

---

## Security Considerations

### Database Access

**Principle of Least Privilege:**

```sql
-- Create dedicated user
CREATE USER opl_operator WITH PASSWORD 'secure-random-password';

-- Grant minimal permissions
GRANT SELECT, UPDATE ON labs TO opl_operator;
GRANT SELECT ON companies TO opl_operator;

-- Explicitly deny destructive operations
REVOKE INSERT, DELETE, TRUNCATE ON labs FROM opl_operator;
REVOKE ALL ON DATABASE opl_labs FROM opl_operator;
```

### Connection Security

**SSL/TLS Configuration:**

- Production: `sslmode=verify-full` with CA certificate
- Staging: `sslmode=require`
- Development: `sslmode=disable` (local only)

**Secret Management:**

- Database credentials stored in Kubernetes Secret
- Mounted as environment variables (not files)
- Rotate credentials periodically
- Use strong random passwords (min 32 characters)

### Network Policies

**Restrict Egress:**

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: database-sync-egress
  namespace: opl-cluster-operator-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
  - Egress
  egress:
  # Allow PostgreSQL
  - to:
    - podSelector:
        matchLabels:
          app: postgres
    ports:
    - protocol: TCP
      port: 5432
  # Allow Kubernetes API
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 6443
```

---

## Monitoring & Observability

### Prometheus Metrics

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    labsSyncedTotal = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "opl_labs_synced_total",
        Help: "Total number of labs successfully synced to Kubernetes",
    })

    labsSyncErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
        Name: "opl_labs_sync_errors_total",
        Help: "Total number of lab sync errors by type",
    }, []string{"error_type"})  // validation, duplicate, database, kubernetes

    dbConnectionStatus = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "opl_database_connection_status",
        Help: "Database connection status (1=connected, 0=disconnected)",
    })

    labsProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
        Name:    "opl_labs_processing_duration_seconds",
        Help:    "Time taken to process a batch of labs",
        Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
    })

    dbQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "opl_database_query_duration_seconds",
        Help:    "Database query latency",
        Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
    }, []string{"query_type"})  // get_unsynced, mark_synced, update_status
)
```

### Health Checks

```go
// Add database health check to manager
if err := mgr.AddHealthzCheck("database", func(req *http.Request) error {
    ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
    defer cancel()
    return dbClient.Ping(ctx)
}); err != nil {
    setupLog.Error(err, "unable to add database health check")
    os.Exit(1)
}
```

### Logging

**Structured Logging with Context:**

```go
logger := log.FromContext(ctx).WithValues(
    "labID", lab.ID,
    "clusterName", lab.ClusterName,
    "state", lab.State,
)

logger.Info("Processing lab")
logger.Error(err, "Failed to create ClusterRequest")
logger.V(1).Info("Debug: Idempotency check passed")  // Verbose logging
```

---

## Performance Considerations

### Polling Interval Tuning

| Interval | Latency | DB Load | Use Case |
|----------|---------|---------|----------|
| 10s | Very low | High | Real-time requirements |
| 30s | Low | Medium | **Recommended for production** |
| 60s | Medium | Low | High-volume deployments |
| 300s | High | Very low | Batch processing |

**Recommendation**: Start with 30s, tune based on metrics

### Batch Size Tuning

| Batch Size | Memory | Throughput | Risk |
|------------|--------|------------|------|
| 10 | Very low | Low | None |
| 100 | Low | Medium | **Recommended** |
| 1000 | Medium | High | OOM if labs are large |
| Unlimited | High | Very high | OOM risk |

**Recommendation**: 100 labs/batch, configurable

### Database Index Strategy

```sql
-- Partial index for unsynced labs (primary query)
CREATE INDEX idx_labs_k8s_synced
ON labs(k8s_synced, created_at)
WHERE k8s_synced = false;

-- Index for lookup by lab ID (status updates)
-- Already covered by PRIMARY KEY on id

-- Composite index for filtering by state and sync status
CREATE INDEX idx_labs_state_synced
ON labs(state, k8s_synced)
WHERE k8s_synced = false;
```

### Connection Pool Sizing

**Formula**: `max_conns = (CPU cores) * 2 + (effective_spindle_count)`

For typical deployment:
- VM: 4 vCPUs
- Disk: SSD (no spindles)
- **Max connections**: 4 * 2 + 0 = 8-10 connections

---

## Rollout Strategy

### Phase 1: Development Testing (Week 1-2)

1. Deploy to dev environment
2. Manually insert test labs in database
3. Verify CR creation and status sync
4. Test error scenarios (duplicates, invalid data)

### Phase 2: Staging Validation (Week 3)

1. Deploy to staging environment with production database snapshot
2. Enable sync for subset of labs (filter by lab ID range)
3. Monitor metrics and logs
4. Performance testing with batch processing

### Phase 3: Production Rollout (Week 4)

1. Deploy during maintenance window
2. Start with polling interval = 5 minutes (conservative)
3. Monitor for 24 hours
4. Gradually reduce interval to 30s
5. Enable status sync (K8s → DB)

### Rollback Plan

If issues occur:
1. Scale operator to 0 replicas: `kubectl scale deployment controller-manager --replicas=0`
2. Database state preserved (k8s_synced flags remain)
3. Manually create CRs if needed
4. Fix issue and redeploy
5. Resume: controller will process remaining unsynced labs

---

## Future Enhancements

### 1. Change Data Capture (CDC)

Replace polling with real-time database events:

```
PostgreSQL → Debezium → Kafka → Operator Event Handler
```

**Benefits:**
- Sub-second latency
- Reduced database load
- Event sourcing for audit

**Trade-offs:**
- More complex infrastructure
- Additional dependencies

### 2. Webhook Integration

Expose HTTP endpoint for Buffalo app to trigger sync:

```go
POST /api/v1/sync/lab/123
```

**Benefits:**
- Immediate sync (no polling delay)
- Buffalo app controls timing

**Trade-offs:**
- Requires ingress/route
- Authentication needed
- Additional security surface

### 3. Reconciliation Report

Daily reconciliation job to detect drift:

```
CronJob:
1. Query all labs with k8s_synced = true
2. Verify corresponding CRs exist
3. Report orphaned labs (synced but CR deleted)
4. Report orphaned CRs (CR exists but lab not in DB)
```

### 4. Multi-Database Support

Support multiple databases for multi-tenant deployments:

```yaml
databases:
  - name: tenant-a
    host: postgres-a.example.com
    namespace: tenant-a
  - name: tenant-b
    host: postgres-b.example.com
    namespace: tenant-b
```

---

## Dependencies

### Go Modules

```
require (
    github.com/jackc/pgx/v5 v5.5.1
    github.com/testcontainers/testcontainers-go v0.27.0
    github.com/pashagolub/pgxmock/v3 v3.3.0
    sigs.k8s.io/controller-runtime v0.21.0
    k8s.io/apimachinery v0.33.0
    k8s.io/client-go v0.33.0
)
```

### External Services

- PostgreSQL 12+ (tested with PostgreSQL 15)
- Kubernetes 1.25+ / OpenShift 4.12+

---

## Summary

This design document specifies a robust, production-ready database synchronization controller that:

✅ **Automates** lab-to-cluster provisioning workflow
✅ **Maintains consistency** between database and Kubernetes
✅ **Handles errors** gracefully with retries and validation
✅ **Scales** with connection pooling and batch processing
✅ **Monitors** with Prometheus metrics and health checks
✅ **Secures** with least-privilege database access and SSL/TLS

**Implementation Effort**: 3-4 weeks for production-ready solution

---

**Document Version:** 1.0
**Date:** 2026-02-02
**Author:** OPL Team
**Status:** Design Review
