# CLAUDE.md

Never ever use emojis in the code base and documentation except alloy or OTel icon.

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This project creates a Kubernetes Custom Resource Definition (CRD) and controller for managing Grafana Cloud Fleet Management Pipelines. It enables declarative management of Fleet Management configuration pipelines as native Kubernetes objects.

### What is Fleet Management?

Grafana Fleet Management is a Grafana Cloud service for managing Alloy and OpenTelemetry collector deployments at scale.

**Pipelines** are standalone configuration fragments (Alloy or OTEL config) that are:
- Created and managed centrally in Fleet Management
- Assigned to collectors based on attribute matchers
- Dynamically loaded by collectors without restart
- Isolated from each other and from local configuration

**Key concepts:**
- **Matchers**: Prometheus Alertmanager-style label selectors (e.g., `collector.os=linux`, `team!=team-a`) used to assign pipelines to collectors
- **Attributes**: Key-value pairs on collectors used for matching (e.g., `environment=production`, `region=us-west`)
- **Hybrid Configuration**: Collectors run local and remote configs concurrently in isolated component controllers
- **Revision History**: Every pipeline create/update/delete is tracked with full snapshots

### Project Goal

Build a Kubernetes controller that allows users to define Fleet Management pipelines as Kubernetes resources, with the controller syncing them to the Fleet Management API.

## Fleet Management Pipeline API

### Base URL

`https://fleet-management-<CLUSTER_NAME>.grafana.net/pipeline.v1.PipelineService/`

Authentication: Basic auth with username and password/token

### Rate Limits

Management endpoints: 3 req/s (requests_per_second:api limit)

### Pipeline Object Structure

```json
{
  "name": "string (required, unique identifier)",
  "contents": "string (required, Alloy or OTEL config)",
  "matchers": ["collector.os=linux", "team!=team-a"],
  "enabled": true,
  "id": "server-assigned",
  "config_type": "CONFIG_TYPE_ALLOY | CONFIG_TYPE_OTEL",
  "source": {
    "type": "SOURCE_TYPE_GIT | SOURCE_TYPE_TERRAFORM | SOURCE_TYPE_UNSPECIFIED",
    "namespace": "string (required if type set)"
  },
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

**Important field notes:**
- Protobuf uses snake_case; JSON responses use camelCase; both accepted in requests
- `name`: Unique identifier for the pipeline
- `contents`: Must be properly escaped JSON string
- `matchers`: 200 character limit per matcher, Prometheus Alertmanager syntax
- `config_type`: Two values supported:
  - `CONFIG_TYPE_ALLOY`: For Grafana Alloy configuration syntax
  - `CONFIG_TYPE_OTEL`: For OpenTelemetry Collector configuration syntax
  - Defaults to CONFIG_TYPE_ALLOY if unspecified
- `id`: Server-assigned, use this for updates/deletes
- `source`: Optional metadata about origin (Git, Terraform, etc.)

### API Operations

**CreatePipeline**
- Creates new pipeline
- Returns 409 if name already exists
- Supports `validate_only: true` for dry-run validation

**UpdatePipeline**
- Updates existing pipeline by ID
- Returns 404 if pipeline doesn't exist
- **IMPORTANT**: Unset fields are removed (not preserved)
- Supports `validate_only: true` for dry-run validation

**UpsertPipeline**
- Creates new or updates existing pipeline
- Idempotent operation (recommended for controllers)
- Unset fields are removed on updates
- Supports `validate_only: true` for dry-run validation

**DeletePipeline**
- Deletes pipeline by ID
- Returns empty response on success
- Returns 404 if not found

**GetPipeline**
- Retrieves pipeline by ID
- Returns full pipeline object with contents

**GetPipelineID**
- Retrieves pipeline ID by name
- Useful for looking up ID from name

**ListPipelines**
- Returns all pipelines matching filters
- Filters: `local_attributes`, `remote_attributes`, `config_type`, `enabled`
- Returns full pipeline objects including contents

**SyncPipelines**
- Bulk create/update/delete from common source
- Atomic operation for GitOps workflows
- Creates pipelines not in Fleet Management
- Updates pipelines that exist with changes
- Deletes pipelines not in request but exist with same source
- All pipelines must share same `source` metadata

### Revision Tracking

**ListPipelinesRevisions**
- Returns all pipeline changes across all pipelines in chronological order
- Omits pipeline contents for performance
- Shows operation: INSERT, UPDATE, DELETE

**ListPipelineRevisions**
- Returns all revisions for a single pipeline by ID
- Includes full pipeline snapshot with contents
- Shows operation that created each revision

**GetPipelineRevision**
- Returns a single revision by revision_id
- Includes full pipeline snapshot with contents

**PipelineRevision structure:**
```json
{
  "revision_id": "server-assigned",
  "snapshot": { /* full Pipeline object */ },
  "created_at": "timestamp",
  "operation": "INSERT | UPDATE | DELETE"
}
```

## CRD Design

### Pipeline CRD Spec

```go
type PipelineSpec struct {
    // Name of the pipeline (unique identifier in Fleet Management)
    // If not specified, uses metadata.name
    Name string `json:"name,omitempty"`

    // Contents of the pipeline configuration (Alloy or OTEL)
    Contents string `json:"contents"`

    // Matchers to assign pipeline to collectors
    // Prometheus Alertmanager syntax: key=value, key!=value, key=~regex, key!~regex
    // +kubebuilder:validation:MaxItems=100
    Matchers []string `json:"matchers,omitempty"`

    // Whether the pipeline is enabled
    // +kubebuilder:default=true
    Enabled bool `json:"enabled"`

    // Type of configuration
    // +kubebuilder:validation:Enum=Alloy;OpenTelemetryCollector
    // +kubebuilder:default=Alloy
    ConfigType ConfigType `json:"configType,omitempty"`

    // Source of the pipeline (for tracking origin)
    Source *PipelineSource `json:"source,omitempty"`
}

type PipelineStatus struct {
    // Server-assigned pipeline ID
    ID string `json:"id,omitempty"`

    // Standard Kubernetes conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Generation of spec that was last reconciled
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // Timestamps from Fleet Management
    CreatedAt *metav1.Time `json:"createdAt,omitempty"`
    UpdatedAt *metav1.Time `json:"updatedAt,omitempty"`

    // Current revision ID from Fleet Management
    RevisionID string `json:"revisionId,omitempty"`
}

// ConfigType represents the type of collector configuration
type ConfigType string

const (
    // ConfigTypeAlloy represents Grafana Alloy configuration syntax
    ConfigTypeAlloy ConfigType = "Alloy"

    // ConfigTypeOpenTelemetryCollector represents OpenTelemetry Collector configuration syntax
    ConfigTypeOpenTelemetryCollector ConfigType = "OpenTelemetryCollector"
)

// ToFleetAPI converts CRD ConfigType to Fleet Management API format
func (c ConfigType) ToFleetAPI() string {
    switch c {
    case ConfigTypeAlloy:
        return "CONFIG_TYPE_ALLOY"
    case ConfigTypeOpenTelemetryCollector:
        return "CONFIG_TYPE_OTEL"
    default:
        return "CONFIG_TYPE_ALLOY"
    }
}

// FromFleetAPI converts Fleet Management API format to CRD ConfigType
func ConfigTypeFromFleetAPI(apiType string) ConfigType {
    switch apiType {
    case "CONFIG_TYPE_OTEL":
        return ConfigTypeOpenTelemetryCollector
    case "CONFIG_TYPE_ALLOY":
        return ConfigTypeAlloy
    default:
        return ConfigTypeAlloy
    }
}
```

### ConfigType: Alloy vs OpenTelemetryCollector

**ConfigTypeAlloy** (default):
- For Grafana Alloy collectors
- Uses Alloy configuration syntax (River/HCL-like)
- Supports Alloy-specific components (prometheus.scrape, loki.source, etc.)
- Example: `prometheus.exporter.self "alloy" { }`

**ConfigTypeOpenTelemetryCollector**:
- For OpenTelemetry Collector instances
- Uses OTEL Collector YAML configuration syntax
- Supports standard OTEL receivers, processors, exporters
- Example: `receivers: { otlp: { protocols: { grpc: {} } } }`

**Important**: The configType must match the type of collector that will use this pipeline:
- Alloy collectors can only use CONFIG_TYPE_ALLOY pipelines
- OTEL collectors can only use CONFIG_TYPE_OTEL pipelines
- Mismatched types will cause configuration errors

**Mapping between CRD and API:**

| CRD Value (spec.configType) | Fleet Management API Value | Collector Type | Config Syntax |
|------------------------------|----------------------------|----------------|---------------|
| `Alloy` (default) | `CONFIG_TYPE_ALLOY` | Grafana Alloy | River/HCL-like |
| `OpenTelemetryCollector` | `CONFIG_TYPE_OTEL` | OpenTelemetry Collector | YAML |

**Validation**: Controller should validate that:
- Alloy configs use Alloy syntax (starts with component blocks)
- OTEL configs use valid YAML with receivers/processors/exporters/service sections

**Kubebuilder CRD validation markers:**
```go
// ConfigType represents the type of collector configuration
// +kubebuilder:validation:Enum=Alloy;OpenTelemetryCollector
// +kubebuilder:default=Alloy
type ConfigType string
```

This ensures:
- Only valid values (`Alloy` or `OpenTelemetryCollector`) can be set
- If not specified, defaults to `Alloy`
- Kubernetes API server validates before accepting the resource

### Status Conditions

Use standard Kubernetes condition types:
- **Ready**: Pipeline is successfully synced to Fleet Management
- **Synced**: Last reconciliation succeeded
- **ValidationError**: Pipeline contents failed validation

### Key Design Decisions

**Name handling:**
- Use `metadata.name` as pipeline name if `spec.name` not set
- This allows Kubernetes-native naming
- Consider namespace prefixing for multi-tenancy

**Contents storage:**
- Inline in spec.contents (simple, direct)
- Alternative: ConfigMapRef for large configs (future enhancement)

**Update strategy:**
- Use UpsertPipeline for idempotent reconciliation
- Simpler than managing Create vs Update logic
- Handles pipeline recreation gracefully

**Deletion:**
- Use finalizers to ensure DeletePipeline called before removing CRD
- Handle 404 gracefully (already deleted)

## Controller Architecture

### Reconciliation Loop

```go
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch Pipeline CRD
    // 2. Handle deletion (finalizer logic)
    // 3. Build Fleet Management pipeline object
    // 4. Call UpsertPipeline API
    // 5. Update status with ID and timestamps
    // 6. Update conditions
    // 7. Requeue if needed
}
```

### Finalizer Logic

```go
const pipelineFinalizer = "pipeline.fleetmanagement.grafana.com/finalizer"

// On create/update:
if !controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
    controllerutil.AddFinalizer(pipeline, pipelineFinalizer)
    // Update CRD
}

// On deletion:
if !pipeline.DeletionTimestamp.IsZero() {
    if controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
        // Call DeletePipeline API
        // Remove finalizer if successful or 404
        controllerutil.RemoveFinalizer(pipeline, pipelineFinalizer)
        // Update CRD
    }
}
```

### Error Handling

- **400 Validation Error**: Update status condition with details, don't retry immediately
- **404 Not Found**: On delete, treat as success; on get, pipeline was deleted externally
- **409 Conflict**: On create, pipeline exists (shouldn't happen with Upsert)
- **429 Rate Limit**: Exponential backoff, requeue with delay
- **5xx Server Error**: Retry with exponential backoff

### Rate Limiting

Implement rate limiting in API client:
```go
// Use rate.Limiter with 3 requests per second
limiter := rate.NewLimiter(rate.Limit(3), 1)
limiter.Wait(ctx) // Before each API call
```

### Caching and Optimization

- Use status.observedGeneration to detect spec changes
- Compare hash of contents to avoid unnecessary updates
- Cache server-side state to reduce ListPipelines calls
- Use controller-runtime's caching layer

## Kubernetes Controller Pitfalls and Best Practices

This section covers critical pitfalls to avoid when developing Kubernetes controllers, based on production experience and best practices.

### CRITICAL: Cache Consistency Issues

**Problem**: Informer cache does NOT provide read-your-writes consistency.

When you create/update a resource, the cached client may return stale data immediately after:

```go
// WRONG: Assumes immediate cache update
r.Client.Create(ctx, childResource)
r.Client.Get(ctx, key, childResource) // May return NotFound or stale data!
```

**Impact on this controller:**
- After UpsertPipeline succeeds, the Pipeline status update may be based on stale cache
- Finalizer logic may see stale state during deletion
- Reconciliation may trigger unnecessary API calls

**Solutions:**

1. **Use ObservedGeneration pattern** (already in design):
```go
// Only reconcile if spec changed
if pipeline.Status.ObservedGeneration == pipeline.Generation {
    // Spec hasn't changed, skip Fleet Management API call
    return ctrl.Result{}, nil
}
```

2. **Store reconciliation results in status**:
```go
// After successful UpsertPipeline, store ID and timestamps in status
status.ID = apiResponse.ID
status.UpdatedAt = apiResponse.UpdatedAt
status.ObservedGeneration = pipeline.Generation

// Next reconciliation can compare without external API call
```

3. **Use uncached client for critical reads**:
```go
type PipelineReconciler struct {
    Client        client.Client        // Cached
    UncachedClient client.Client       // Direct API
}

// For critical operations, use uncached client
r.UncachedClient.Get(ctx, key, pipeline)
```

4. **Implement expectations pattern**:
```go
// Track expected state changes
expectations := controller.NewUIDTracker()
expectations.ExpectCreation(namespace, name)

// When create succeeds, mark as observed
expectations.Creation(namespace, name)

// In reconcile, check expectations
if !expectations.SatisfiedExpectations(namespace, name) {
    // Wait for cache to catch up
    return ctrl.Result{RequeueAfter: time.Second}, nil
}
```

### Don't Block Reconcile with Long Operations

**Problem**: Long-running operations in Reconcile() block worker goroutines and increase workqueue depth.

**Bad examples for this controller:**
```go
// WRONG: Synchronous validation with external system
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // This blocks the worker for 30+ seconds
    if r.spec.ValidateWithCollector {
        time.Sleep(30 * time.Second) // Waiting for collector to apply config
        checkCollectorHealth()
    }
}
```

**Solutions:**

1. **Return early and requeue**:
```go
// Check if pipeline was recently updated
if time.Since(status.UpdatedAt.Time) < 30*time.Second {
    // Too soon to check if applied successfully
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

2. **Use status conditions for async operations**:
```go
// Set "Syncing" condition immediately
meta.SetStatusCondition(&pipeline.Status.Conditions, metav1.Condition{
    Type:   "Synced",
    Status: metav1.ConditionUnknown,
    Reason: "SyncInProgress",
})

// Make API call
apiResp, err := r.fleetClient.UpsertPipeline(ctx, req)

// Update to final condition
if err != nil {
    meta.SetStatusCondition(&pipeline.Status.Conditions, metav1.Condition{
        Type:    "Synced",
        Status:  metav1.ConditionFalse,
        Reason:  "SyncFailed",
        Message: err.Error(),
    })
} else {
    meta.SetStatusCondition(&pipeline.Status.Conditions, metav1.Condition{
        Type:   "Synced",
        Status: metav1.ConditionTrue,
        Reason: "SyncSucceeded",
    })
}
```

3. **Monitor workqueue metrics**:
```bash
# Check for growing queue depth
workqueue_depth{name="pipeline"}
workqueue_work_duration_seconds{name="pipeline"}
workqueue_unfinished_work_seconds{name="pipeline"}
```

### Avoid Unnecessary API Calls

**Problem**: Calling external APIs on every reconciliation wastes resources and hits rate limits.

**Bad pattern:**
```go
// WRONG: Always calls Fleet Management API
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // This runs every resync period (default 10h) even if nothing changed!
    apiPipeline, _ := r.fleetClient.GetPipeline(ctx, status.ID)
    r.Client.Update(ctx, pipeline) // Unnecessary status update
}
```

**Good pattern:**
```go
// CORRECT: Only call API when spec changed
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Skip if spec unchanged
    if pipeline.Status.ObservedGeneration == pipeline.Generation {
        return ctrl.Result{}, nil
    }

    // Only upsert when spec actually changed
    apiPipeline, err := r.fleetClient.UpsertPipeline(ctx, buildRequest(pipeline.Spec))

    // Update status only if changed
    if !equality.Semantic.DeepEqual(pipeline.Status, newStatus) {
        pipeline.Status = newStatus
        r.Client.Status().Update(ctx, pipeline)
    }
}
```

**For this controller specifically:**
- Don't call GetPipeline unless debugging (use UpsertPipeline response instead)
- Don't call ListPipelines on every reconcile (too expensive with 3 req/s limit)
- Cache Fleet Management state in controller memory if needed
- Use observedGeneration to skip reconciliation when spec unchanged

### Return Errors, Don't Just Requeue

**Problem**: Using `Result{Requeue: true}` instead of returning errors hides problems.

**Wrong:**
```go
pipeline, err := r.fleetClient.UpsertPipeline(ctx, req)
if err != nil {
    // WRONG: Error is swallowed, no backoff, no log context
    return ctrl.Result{Requeue: true}, nil
}
```

**Correct:**
```go
pipeline, err := r.fleetClient.UpsertPipeline(ctx, req)
if err != nil {
    // CORRECT: Error is returned, automatic exponential backoff
    return ctrl.Result{}, fmt.Errorf("failed to upsert pipeline: %w", err)
}
```

**When to use Requeue: true:**
- External dependency not ready yet (not an error condition)
- Scheduled recheck (e.g., after 30 seconds)
- Rate limiting (use RequeueAfter for specific delay)

**When to return error:**
- API call failures
- Invalid configuration
- Unexpected conditions
- Transient errors that should retry with backoff

### Status Updates and Conflicts

**Problem**: Status updates can fail with conflicts, causing unnecessary retries.

**Critical pattern for this controller:**
```go
// Use Status().Update() not Update()
r.Client.Status().Update(ctx, pipeline)

// Handle conflicts gracefully
if err := r.Client.Status().Update(ctx, pipeline); err != nil {
    if apierrors.IsConflict(err) {
        // Resource was modified, requeue to get fresh copy
        return ctrl.Result{Requeue: true}, nil
    }
    return ctrl.Result{}, err
}

// NEVER update status in spec update - they are separate
r.Client.Update(ctx, pipeline)        // Updates spec + metadata
r.Client.Status().Update(ctx, pipeline) // Updates status only
```

### Finalizer Pitfalls

**Problem**: Incorrect finalizer handling can cause deadlocks or orphaned resources.

**Critical issues:**

1. **Deadlock**: Two resources with finalizers waiting for each other
2. **Orphaned resources**: Finalizer not removed after cleanup
3. **Duplicate cleanup**: Finalizer logic runs multiple times

**Correct pattern for this controller:**
```go
const pipelineFinalizer = "pipeline.fleetmanagement.grafana.com/finalizer"

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    pipeline := &Pipeline{}
    if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
        if apierrors.IsNotFound(err) {
            // Pipeline already deleted, nothing to do
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // Handle deletion
    if !pipeline.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
            // Perform cleanup
            if err := r.deleteFleetPipeline(ctx, pipeline); err != nil {
                if isNotFoundError(err) {
                    // Already deleted in Fleet Management, continue
                } else {
                    // Failed to delete, retry
                    return ctrl.Result{}, err
                }
            }

            // Remove finalizer
            controllerutil.RemoveFinalizer(pipeline, pipelineFinalizer)
            if err := r.Update(ctx, pipeline); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
        controllerutil.AddFinalizer(pipeline, pipelineFinalizer)
        if err := r.Update(ctx, pipeline); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Normal reconciliation logic
    return r.reconcileNormal(ctx, pipeline)
}

func (r *PipelineReconciler) deleteFleetPipeline(ctx context.Context, pipeline *Pipeline) error {
    if pipeline.Status.ID == "" {
        // No ID means never created in Fleet Management
        return nil
    }

    err := r.fleetClient.DeletePipeline(ctx, pipeline.Status.ID)
    if err != nil && isNotFoundError(err) {
        // Already deleted, treat as success
        return nil
    }
    return err
}
```

### Declare All Resource Watches

**Problem**: Undeclared resource queries cause controller-runtime to dynamically initialize informers, leading to unexpected behavior.

**For this controller:**
```go
// Enable strict checking in manager config
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    // Fail if controller queries resources without declaring watches
    Client: client.Options{
        Cache: &client.CacheOptions{
            // This will cause errors if you query resources without watches
            DisableFor: []client.Object{}, // Empty = cache all watched types
        },
    },
})

// In SetupWithManager, declare what you watch
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&Pipeline{}).  // Primary resource
        // If you watch ConfigMaps for pipeline contents:
        Owns(&corev1.ConfigMap{}).
        Complete(r)
}
```

### Structure Reconcile Logic Consistently

**Problem**: Dumping all logic in Reconcile() makes code hard to maintain and test.

**Recommended structure for this controller:**

```go
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // 1. Fetch resource
    pipeline := &Pipeline{}
    if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Handle deletion
    if !pipeline.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, pipeline)
    }

    // 3. Add finalizer if needed
    if !controllerutil.ContainsFinalizer(pipeline, pipelineFinalizer) {
        return r.addFinalizer(ctx, pipeline)
    }

    // 4. Check if reconciliation needed
    if pipeline.Status.ObservedGeneration == pipeline.Generation {
        return ctrl.Result{}, nil
    }

    // 5. Reconcile normal case
    return r.reconcileNormal(ctx, pipeline)
}

func (r *PipelineReconciler) reconcileNormal(ctx context.Context, pipeline *Pipeline) (ctrl.Result, error) {
    // Business logic here
    apiReq := r.buildUpsertRequest(pipeline)
    apiResp, err := r.fleetClient.UpsertPipeline(ctx, apiReq)
    if err != nil {
        return r.handleAPIError(ctx, pipeline, err)
    }

    return r.updateStatus(ctx, pipeline, apiResp)
}

func (r *PipelineReconciler) handleAPIError(ctx context.Context, pipeline *Pipeline, err error) (ctrl.Result, error) {
    if isRateLimitError(err) {
        // Exponential backoff handled by controller-runtime
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    if isValidationError(err) {
        // Update condition and don't retry immediately
        meta.SetStatusCondition(&pipeline.Status.Conditions, metav1.Condition{
            Type:    "Synced",
            Status:  metav1.ConditionFalse,
            Reason:  "ValidationFailed",
            Message: err.Error(),
        })
        r.Status().Update(ctx, pipeline)
        return ctrl.Result{}, nil // Don't requeue
    }

    // Other errors: return for exponential backoff
    return ctrl.Result{}, fmt.Errorf("fleet API error: %w", err)
}
```

### Testing Best Practices

**Use envtest for integration tests:**
```go
import (
    "sigs.k8s.io/controller-runtime/pkg/envtest"
)

var testEnv *envtest.Environment

func TestMain(m *testing.M) {
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
    }

    cfg, err := testEnv.Start()
    // ... setup test clients

    code := m.Run()
    testEnv.Stop()
    os.Exit(code)
}
```

**Mock Fleet Management API:**
```go
type mockFleetClient struct {
    pipelines map[string]*Pipeline
    callCount int
}

func (m *mockFleetClient) UpsertPipeline(ctx context.Context, req *UpsertRequest) (*Pipeline, error) {
    m.callCount++
    m.pipelines[req.Pipeline.Name] = req.Pipeline
    return req.Pipeline, nil
}

func TestReconcile_SkipsWhenNoChange(t *testing.T) {
    mock := &mockFleetClient{}
    reconciler := &PipelineReconciler{fleetClient: mock}

    pipeline := &Pipeline{
        ObjectMeta: metav1.ObjectMeta{Generation: 1},
        Status:     PipelineStatus{ObservedGeneration: 1}, // Already reconciled
    }

    reconciler.Reconcile(ctx, req)

    // Should not call API when generation matches
    assert.Equal(t, 0, mock.callCount)
}
```

### Monitoring and Observability

**Essential metrics for this controller:**

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
    fleetAPICallsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "fleet_api_calls_total",
            Help: "Total Fleet Management API calls",
        },
        []string{"operation", "status"},
    )

    reconcileErrorsTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "pipeline_reconcile_errors_total",
            Help: "Total reconciliation errors",
        },
    )
)

func init() {
    metrics.Registry.MustRegister(fleetAPICallsTotal, reconcileErrorsTotal)
}

// In code:
fleetAPICallsTotal.WithLabelValues("upsert", "success").Inc()
```

**Watch these controller-runtime metrics:**
- `workqueue_depth{name="pipeline"}` - Growing = reconcile too slow
- `workqueue_work_duration_seconds{name="pipeline"}` - Reconcile performance
- `controller_runtime_reconcile_total{result="error"}` - Error rate
- `controller_runtime_reconcile_total{result="requeue"}` - Requeue rate

### Summary Checklist

Before shipping this controller, verify:

- [ ] ObservedGeneration pattern implemented to skip unnecessary reconciles
- [ ] Status updates use Status().Update() not Update()
- [ ] Finalizer logic handles 404 errors gracefully
- [ ] No long-running operations block Reconcile()
- [ ] Errors are returned (not swallowed with Requeue: true)
- [ ] Fleet Management API calls only happen when spec changes
- [ ] Rate limiting implemented in API client (3 req/s)
- [ ] Status conditions follow Kubernetes conventions
- [ ] Metrics and logging for observability
- [ ] Integration tests with envtest
- [ ] Unit tests with mocked Fleet Management API

## Go Best Practices for This Project

This section highlights critical Go idioms and patterns specifically relevant to building the Fleet Management Pipeline controller.

### Formatting and Structure

**Always use gofmt:**
```bash
# Format all files before commit
go fmt ./...

# Or use goimports (also organizes imports)
goimports -w .
```

**Package naming:**
- Package `fleetmanagement/v1alpha1` for CRD types
- Package `controller` for reconciliation logic
- Package `fleetclient` for Fleet Management API client
- Keep names lowercase, single-word where possible

**File organization:**
```
internal/controller/
├── pipeline_controller.go      # Main Reconcile logic
├── pipeline_controller_test.go # Tests
├── pipeline_finalizer.go       # Finalizer handling
└── pipeline_status.go          # Status update helpers
```

### Error Handling

**Return errors, don't panic:**
```go
// CORRECT: Return errors for caller to handle
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    pipeline := &Pipeline{}
    if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    apiResp, err := r.fleetClient.UpsertPipeline(ctx, buildRequest(pipeline))
    if err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to upsert pipeline: %w", err)
    }

    return ctrl.Result{}, nil
}

// WRONG: Don't panic in controller code
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    pipeline := &Pipeline{}
    if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
        panic(err) // NEVER DO THIS
    }
}
```

**Wrap errors with context:**
```go
// Use %w for error wrapping (Go 1.13+)
if err := r.fleetClient.UpsertPipeline(ctx, req); err != nil {
    return fmt.Errorf("upsert pipeline %s/%s: %w", pipeline.Namespace, pipeline.Name, err)
}

// Caller can use errors.Is() or errors.As()
if errors.Is(err, ErrRateLimit) {
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}
```

**Custom error types for API errors:**
```go
type FleetAPIError struct {
    StatusCode int
    Operation  string
    Message    string
}

func (e *FleetAPIError) Error() string {
    return fmt.Sprintf("%s failed (HTTP %d): %s", e.Operation, e.StatusCode, e.Message)
}

// In controller:
if apiErr, ok := err.(*FleetAPIError); ok {
    if apiErr.StatusCode == 400 {
        // Validation error - update status condition
        meta.SetStatusCondition(&pipeline.Status.Conditions, metav1.Condition{
            Type:    "Synced",
            Status:  metav1.ConditionFalse,
            Reason:  "ValidationFailed",
            Message: apiErr.Message,
        })
        return ctrl.Result{}, nil // Don't retry
    }
}
```

### Defer for Resource Cleanup

**Use defer for cleanup:**
```go
func (c *FleetClient) UpsertPipeline(ctx context.Context, req *UpsertPipelineRequest) (*Pipeline, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, err
    }

    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"UpsertPipeline", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close() // ALWAYS defer Close()

    if resp.StatusCode != http.StatusOK {
        // Read error message
        body, _ := io.ReadAll(resp.Body)
        return nil, &FleetAPIError{
            StatusCode: resp.StatusCode,
            Operation:  "UpsertPipeline",
            Message:    string(body),
        }
    }

    var pipeline Pipeline
    if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
        return nil, err
    }

    return &pipeline, nil
}
```

**Defer for tracing/logging:**
```go
func (r *PipelineReconciler) reconcileNormal(ctx context.Context, pipeline *Pipeline) (result ctrl.Result, err error) {
    log := log.FromContext(ctx)

    // Log on function exit
    defer func() {
        if err != nil {
            log.Error(err, "reconciliation failed")
        } else {
            log.Info("reconciliation succeeded")
        }
    }()

    // Business logic here
    return r.doReconcile(ctx, pipeline)
}
```

### Interfaces and Composition

**Define interfaces in consumer package:**
```go
// In internal/controller/pipeline_controller.go
// Define what the controller needs, not what client provides
type FleetPipelineClient interface {
    UpsertPipeline(ctx context.Context, req *UpsertPipelineRequest) (*Pipeline, error)
    DeletePipeline(ctx context.Context, id string) error
    GetPipeline(ctx context.Context, id string) (*Pipeline, error)
}

type PipelineReconciler struct {
    client.Client
    FleetClient FleetPipelineClient // Interface, not concrete type
    Scheme      *runtime.Scheme
}

// Easy to mock for testing
type mockFleetClient struct {
    pipelines map[string]*Pipeline
}

func (m *mockFleetClient) UpsertPipeline(ctx context.Context, req *UpsertPipelineRequest) (*Pipeline, error) {
    m.pipelines[req.Pipeline.Name] = req.Pipeline
    return req.Pipeline, nil
}
```

**Use embedding for composition:**
```go
// Embed client.Client to get all methods
type PipelineReconciler struct {
    client.Client // Embedded - all methods available directly
    Scheme *runtime.Scheme
    FleetClient FleetPipelineClient
}

// Can call r.Get(), r.Update() directly without r.Client.Get()
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    pipeline := &Pipeline{}
    if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil { // Not r.Client.Get()
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    // ...
}
```

**Verify interface implementation at compile time:**
```go
// At package level - ensures PipelineReconciler implements reconcile.Reconciler
var _ reconcile.Reconciler = &PipelineReconciler{}

// Ensures FleetClient implements our interface
var _ FleetPipelineClient = &FleetClient{}
```

### Pointers vs Values

**Use pointers for:**
- Large structs (avoid copying)
- Structs that need modification
- Kubernetes objects (always pointers)

**Use values for:**
- Small structs (< 64 bytes)
- Immutable data
- When copy semantics are desired

**For this controller:**
```go
// CRD types: Always pointers
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    pipeline := &fleetmanagementv1alpha1.Pipeline{} // Pointer
    if err := r.Get(ctx, req.NamespacedName, pipeline); err != nil {
        return ctrl.Result{}, err
    }
}

// API request/response types: Use pointers for consistency
type UpsertPipelineRequest struct {
    Pipeline     *Pipeline `json:"pipeline"`
    ValidateOnly bool      `json:"validateOnly,omitempty"`
}

// Small config structs: Can use values
type RateLimitConfig struct {
    RequestsPerSecond int
    BurstSize         int
}
```

### Slices and Maps

**Initialize with make for known sizes:**
```go
// If you know size, pre-allocate
matchers := make([]string, 0, len(pipeline.Spec.Matchers))
for _, m := range pipeline.Spec.Matchers {
    matchers = append(matchers, m)
}

// Maps: always use make
conditions := make(map[string]metav1.Condition)
```

**Check map presence with comma-ok:**
```go
// Check if key exists
if pipeline, ok := r.cache[name]; ok {
    return pipeline
}

// Distinguish zero value from missing
count, exists := counters[key]
if !exists {
    counters[key] = 1
} else {
    counters[key] = count + 1
}
```

**Range over slices - be careful with pointers:**
```go
// WRONG: All pointers reference same variable
var pipelines []*Pipeline
for _, p := range pipelineSlice {
    pipelines = append(pipelines, &p) // BUG: All point to same memory!
}

// CORRECT: Take address of slice element
for i := range pipelineSlice {
    pipelines = append(pipelines, &pipelineSlice[i])
}

// Or copy value
for _, p := range pipelineSlice {
    pCopy := p
    pipelines = append(pipelines, &pCopy)
}
```

### Goroutines and Channels

**Don't start goroutines in controller Reconcile:**
```go
// WRONG: Goroutines in Reconcile can cause race conditions
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    go r.fleetClient.UpsertPipeline(ctx, req) // DANGEROUS!
    return ctrl.Result{}, nil
}

// CORRECT: Synchronous operations in Reconcile
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    _, err := r.fleetClient.UpsertPipeline(ctx, req) // Synchronous
    return ctrl.Result{}, err
}
```

**Use channels for signaling in API client:**
```go
// Use channel to coordinate shutdown
type FleetClient struct {
    httpClient *http.Client
    done       chan struct{}
}

func (c *FleetClient) Start() {
    go c.backgroundRefresh()
}

func (c *FleetClient) Stop() {
    close(c.done)
}

func (c *FleetClient) backgroundRefresh() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.refreshCache()
        case <-c.done:
            return
        }
    }
}
```

**Use context for cancellation:**
```go
func (c *FleetClient) UpsertPipeline(ctx context.Context, req *UpsertPipelineRequest) (*Pipeline, error) {
    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, body)
    if err != nil {
        return nil, err
    }

    // Respects context cancellation/timeout
    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // ...
}
```

### Constants and Enumerations

**Use string constants for enumerations (for CRDs):**
```go
// ConfigType represents the type of collector configuration
// +kubebuilder:validation:Enum=Alloy;OpenTelemetryCollector
type ConfigType string

const (
    ConfigTypeAlloy                  ConfigType = "Alloy"
    ConfigTypeOpenTelemetryCollector ConfigType = "OpenTelemetryCollector"
)

// ToFleetAPI converts CRD ConfigType to Fleet Management API format
func (c ConfigType) ToFleetAPI() string {
    switch c {
    case ConfigTypeAlloy:
        return "CONFIG_TYPE_ALLOY"
    case ConfigTypeOpenTelemetryCollector:
        return "CONFIG_TYPE_OTEL"
    default:
        return "CONFIG_TYPE_ALLOY" // Default to Alloy
    }
}

// String implements fmt.Stringer
func (c ConfigType) String() string {
    return string(c)
}
```

**Group related constants:**
```go
const (
    // Finalizer name
    pipelineFinalizer = "pipeline.fleetmanagement.grafana.com/finalizer"

    // Condition types
    conditionTypeReady  = "Ready"
    conditionTypeSynced = "Synced"

    // Annotation keys
    annotationValidateOnly = "fleetmanagement.grafana.com/validate-only"
    annotationPipelineID   = "fleetmanagement.grafana.com/pipeline-id"
)
```

### Testing Patterns

**Table-driven tests:**
```go
func TestBuildUpsertRequest(t *testing.T) {
    tests := []struct {
        name     string
        pipeline *Pipeline
        want     *UpsertPipelineRequest
        wantErr  bool
    }{
        {
            name: "basic pipeline",
            pipeline: &Pipeline{
                Spec: PipelineSpec{
                    Name:     "test",
                    Contents: "config",
                    Enabled:  true,
                },
            },
            want: &UpsertPipelineRequest{
                Pipeline: &FleetPipeline{
                    Name:     "test",
                    Contents: "config",
                    Enabled:  true,
                },
            },
            wantErr: false,
        },
        {
            name: "empty contents",
            pipeline: &Pipeline{
                Spec: PipelineSpec{
                    Name: "test",
                },
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := buildUpsertRequest(tt.pipeline)
            if (err != nil) != tt.wantErr {
                t.Errorf("buildUpsertRequest() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("buildUpsertRequest() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Mock interfaces with testify:**
```go
import "github.com/stretchr/testify/mock"

type MockFleetClient struct {
    mock.Mock
}

func (m *MockFleetClient) UpsertPipeline(ctx context.Context, req *UpsertPipelineRequest) (*Pipeline, error) {
    args := m.Called(ctx, req)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Pipeline), args.Error(1)
}

func TestReconcile_Success(t *testing.T) {
    mockClient := new(MockFleetClient)
    mockClient.On("UpsertPipeline", mock.Anything, mock.Anything).Return(&Pipeline{ID: "123"}, nil)

    reconciler := &PipelineReconciler{
        FleetClient: mockClient,
    }

    // Test reconciliation
    result, err := reconciler.Reconcile(ctx, req)

    assert.NoError(t, err)
    assert.Equal(t, ctrl.Result{}, result)
    mockClient.AssertExpectations(t)
}
```

### JSON Handling

**Use struct tags for JSON marshaling:**
```go
type Pipeline struct {
    Name      string   `json:"name"`
    Contents  string   `json:"contents"`
    Matchers  []string `json:"matchers,omitempty"`  // Omit if empty
    Enabled   bool     `json:"enabled"`
    ID        string   `json:"id,omitempty"`
    CreatedAt *metav1.Time `json:"createdAt,omitempty"` // Pointer for optional fields
}
```

**Handle both snake_case and camelCase from API:**
```go
// API returns camelCase but accepts both
type Pipeline struct {
    Name     string `json:"name"`
    Contents string `json:"contents"`
    // Will accept both createdAt and created_at
    CreatedAt *time.Time `json:"createdAt,omitempty"`
}
```

**Custom JSON marshaling for special cases:**
```go
type ConfigType string

const (
    ConfigTypeAlloy ConfigType = "CONFIG_TYPE_ALLOY"
    ConfigTypeOTEL  ConfigType = "CONFIG_TYPE_OTEL"
)

func (c ConfigType) MarshalJSON() ([]byte, error) {
    return json.Marshal(string(c))
}

func (c *ConfigType) UnmarshalJSON(data []byte) error {
    var s string
    if err := json.Unmarshal(data, &s); err != nil {
        return err
    }
    *c = ConfigType(s)
    return nil
}
```

### Initialization and Constructors

**Use New functions for constructors:**
```go
// Constructor for API client
func NewFleetClient(baseURL, username, password string) *FleetClient {
    return &FleetClient{
        baseURL:  baseURL,
        username: username,
        password: password,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:    100,
                IdleConnTimeout: 90 * time.Second,
            },
        },
        limiter: rate.NewLimiter(rate.Limit(3), 1),
    }
}
```

**Use init() for package-level setup:**
```go
var (
    fleetAPICallsTotal *prometheus.CounterVec
    reconcileErrorsTotal prometheus.Counter
)

func init() {
    fleetAPICallsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "fleet_api_calls_total",
            Help: "Total Fleet Management API calls",
        },
        []string{"operation", "status"},
    )

    reconcileErrorsTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "pipeline_reconcile_errors_total",
            Help: "Total reconciliation errors",
        },
    )

    metrics.Registry.MustRegister(fleetAPICallsTotal, reconcileErrorsTotal)
}
```

### String Formatting and Logging

**Use structured logging:**
```go
import "sigs.k8s.io/controller-runtime/pkg/log"

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Structured logging with key-value pairs
    log.Info("reconciling pipeline",
        "namespace", req.Namespace,
        "name", req.Name)

    // Add values to context for nested calls
    ctx = log.IntoContext(ctx)

    // Error logging
    if err != nil {
        log.Error(err, "failed to upsert pipeline",
            "pipelineID", pipeline.Status.ID,
            "generation", pipeline.Generation)
    }
}
```

**Don't use fmt.Sprintf for errors:**
```go
// WRONG: String concatenation loses error wrapping
return fmt.Errorf(fmt.Sprintf("failed: %s", err.Error()))

// CORRECT: Use %w for error wrapping
return fmt.Errorf("failed to process: %w", err)
```

### Type Assertions and Type Switches

**Safe type assertions:**
```go
// Check if error is specific type
if apiErr, ok := err.(*FleetAPIError); ok {
    switch apiErr.StatusCode {
    case 400:
        // Handle validation error
    case 404:
        // Handle not found
    case 429:
        // Handle rate limit
    }
}

// Type switch for multiple types
switch err := err.(type) {
case *FleetAPIError:
    return handleAPIError(err)
case *net.Error:
    return handleNetworkError(err)
case nil:
    return nil
default:
    return fmt.Errorf("unexpected error type: %T", err)
}
```

### Go Module Best Practices

**go.mod organization:**
```go
module github.com/yourorg/fm-crd

go 1.21

require (
    k8s.io/api v0.28.0
    k8s.io/apimachinery v0.28.0
    k8s.io/client-go v0.28.0
    sigs.k8s.io/controller-runtime v0.16.0
)

require (
    // Indirect dependencies
    golang.org/x/time v0.3.0 // indirect
)
```

**Useful commands:**
```bash
# Update dependencies
go get -u ./...

# Tidy dependencies (remove unused)
go mod tidy

# Vendor dependencies
go mod vendor

# Verify checksums
go mod verify
```

### Summary: Go Patterns for This Controller

**Critical patterns to follow:**
1. Return errors, don't panic
2. Use defer for cleanup (Close, Unlock)
3. Define interfaces in consumer package
4. Use pointers for Kubernetes objects
5. Check map presence with comma-ok
6. Use context for cancellation
7. Table-driven tests
8. Structured logging
9. Error wrapping with %w
10. Verify interface implementation at compile time

**Avoid:**
1. Starting goroutines in Reconcile
2. Mutating loop variables in range
3. Ignoring errors
4. Using fmt.Sprintf for errors
5. Not deferring Close()
6. Type assertions without ok check
7. String concatenation for errors

## Development Workflow

### Project Structure

```
api/v1alpha1/
├── pipeline_types.go       # CRD definition
├── groupversion_info.go    # API group metadata
└── zz_generated.deepcopy.go # Generated

internal/controller/
└── pipeline_controller.go  # Reconciliation logic

pkg/client/
├── client.go              # Fleet Management API client
└── types.go               # API request/response types

config/
├── crd/                   # Generated CRD manifests
│   └── bases/
├── rbac/                  # RBAC roles
├── manager/               # Controller deployment
└── samples/               # Example Pipeline CRs
```

### Setup Project

```bash
# Initialize with kubebuilder
kubebuilder init --domain grafana.com --repo github.com/yourorg/fm-crd

# Create Pipeline API
kubebuilder create api --group fleetmanagement --version v1alpha1 --kind Pipeline
```

### Common Commands

```bash
# Generate code (DeepCopy, clientset, etc.)
make generate

# Generate CRD manifests
make manifests

# Run tests
make test

# Format and lint
make fmt
make vet

# Install CRDs to cluster
make install

# Run controller locally (against current kubeconfig cluster)
make run

# Build and deploy to cluster
make docker-build IMG=<registry>/fleet-management-operator:tag
make docker-push IMG=<registry>/fleet-management-operator:tag
make deploy IMG=<registry>/fleet-management-operator:tag

# Remove from cluster
make undeploy
```

### Testing Strategy

**Unit tests:**
- Mock Fleet Management API responses
- Test reconciliation logic with fake K8s client
- Test finalizer handling
- Test error scenarios

**Integration tests:**
- Use envtest (controller-runtime test framework)
- Test full reconciliation loop
- Verify CRD status updates
- Test API client with recorded responses

**E2E tests:**
- Deploy to real cluster
- Test against real Fleet Management API (requires credentials)
- Verify collectors receive configuration

## Fleet Management API Client

### Client Structure

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
    limiter    *rate.Limiter
    username   string
    password   string
}

func (c *Client) UpsertPipeline(ctx context.Context, req *UpsertPipelineRequest) (*Pipeline, error)
func (c *Client) DeletePipeline(ctx context.Context, id string) error
func (c *Client) GetPipeline(ctx context.Context, id string) (*Pipeline, error)
func (c *Client) GetPipelineID(ctx context.Context, name string) (string, error)
func (c *Client) ListPipelines(ctx context.Context, req *ListPipelinesRequest) ([]*Pipeline, error)
```

### HTTP Client Configuration

```go
httpClient := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 10 * time.Second,
    },
}
```

### Request Construction

```go
// Build request
reqBody, _ := json.Marshal(request)
req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
req.Header.Set("Content-Type", "application/json")
req.SetBasicAuth(c.username, c.password)

// Rate limit
c.limiter.Wait(ctx)

// Execute
resp, _ := c.httpClient.Do(req)
```

### Response Parsing

```go
// Parse response (uses camelCase)
var pipeline Pipeline
if err := json.NewDecoder(resp.Body).Decode(&pipeline); err != nil {
    return nil, err
}

// Map to CRD status
status.ID = pipeline.ID
status.CreatedAt = (*metav1.Time)(pipeline.CreatedAt)
status.UpdatedAt = (*metav1.Time)(pipeline.UpdatedAt)
```

## Configuration and Credentials

### Controller Configuration

Store Fleet Management credentials in a Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: fleet-management-credentials
  namespace: fm-controller-system
type: Opaque
stringData:
  base-url: https://fleet-management-prod-001.grafana.net/pipeline.v1.PipelineService/
  username: "12345"
  password: "glc_xxxxx"
```

Mount in controller deployment:
```yaml
env:
  - name: FLEET_MANAGEMENT_BASE_URL
    valueFrom:
      secretKeyRef:
        name: fleet-management-credentials
        key: base-url
  - name: FLEET_MANAGEMENT_USERNAME
    valueFrom:
      secretKeyRef:
        name: fleet-management-credentials
        key: username
  - name: FLEET_MANAGEMENT_PASSWORD
    valueFrom:
      secretKeyRef:
        name: fleet-management-credentials
        key: password
```

### Multi-tenant Support (Future)

Options for supporting multiple Fleet Management instances:
1. Per-Pipeline credentials via annotation
2. Per-namespace credentials via Secret reference
3. Global default with override capability

## Important API Behaviors

### Field Name Conventions

- Protobuf definitions use snake_case
- JSON responses use camelCase
- Both formats accepted in requests
- Go structs should use json tags for both

```go
type Pipeline struct {
    Name      string   `json:"name"`
    Contents  string   `json:"contents"`
    Matchers  []string `json:"matchers,omitempty"`
    Enabled   bool     `json:"enabled"`
    ID        string   `json:"id,omitempty"`
    CreatedAt *time.Time `json:"createdAt,omitempty" json:"created_at,omitempty"`
    UpdatedAt *time.Time `json:"updatedAt,omitempty" json:"updated_at,omitempty"`
}
```

### Update Semantics

**Critical behavior**: UpdatePipeline and UpsertPipeline are NOT selective.

If you omit a field in the request, it's removed from the pipeline:

```json
// Current pipeline
{"name": "test", "contents": "...", "enabled": true, "matchers": ["env=prod"]}

// Update request (missing matchers)
{"pipeline": {"name": "test", "contents": "...", "enabled": true}}

// Result: matchers are REMOVED
{"name": "test", "contents": "...", "enabled": true, "matchers": []}
```

**Controller implication**: Always include all fields from spec when calling Upsert.

### Matcher Syntax

Follows Prometheus Alertmanager syntax:

- `key=value` - Equals
- `key!=value` - Not equals
- `key=~regex` - Regex match
- `key!~regex` - Regex not match

Examples:
- `collector.os=linux` - Matches Linux collectors
- `environment!=development` - Excludes development
- `team=~team-(a|b)` - Matches team-a or team-b
- `region!~us-.*` - Excludes US regions

Constraints:
- 200 character limit per matcher
- Matchers are AND'd together (all must match)
- Multiple pipelines can match same collector

### Configuration Content Escaping

Pipeline contents must be properly escaped for JSON:

```bash
# Use jq to properly escape
jq --arg contents "$(cat config.alloy)" \
   --arg name "myname" \
   --argjson matchers '["collector.os=linux"]' \
   --argjson enabled true \
   '.pipeline = {name: $name, contents: $contents, matchers: $matchers, enabled: $enabled}' \
   <<< '{}'
```

In Go:
```go
// json.Marshal automatically escapes
pipeline := &Pipeline{
    Name:     "test",
    Contents: string(configFileBytes), // Will be escaped
    Matchers: []string{"env=prod"},
    Enabled:  true,
}
body, _ := json.Marshal(map[string]interface{}{"pipeline": pipeline})
```

### Validation

Use `validate_only: true` to test configuration without applying:

```go
req := &UpsertPipelineRequest{
    Pipeline: pipeline,
    ValidateOnly: true,
}
resp, err := client.UpsertPipeline(ctx, req)
// If err is nil, configuration is valid
// resp shows what would be created/updated
```

**Client-side validation in controller:**

Before sending to Fleet Management API, validate that configType matches content:

```go
func validatePipelineConfig(pipeline *Pipeline) error {
    configType := pipeline.Spec.ConfigType
    contents := pipeline.Spec.Contents

    switch configType {
    case ConfigTypeAlloy, "": // Empty defaults to Alloy
        // Basic Alloy syntax check - should start with component blocks
        if !looksLikeAlloyConfig(contents) {
            return fmt.Errorf("configType is Alloy but contents don't match Alloy syntax")
        }
    case ConfigTypeOpenTelemetryCollector:
        // Basic OTEL syntax check - should be valid YAML with required sections
        if !looksLikeOTELConfig(contents) {
            return fmt.Errorf("configType is OpenTelemetryCollector but contents don't match OTEL syntax")
        }
    default:
        return fmt.Errorf("invalid configType: %s", configType)
    }

    return nil
}

func looksLikeAlloyConfig(contents string) bool {
    // Alloy configs typically start with component blocks
    // e.g., "prometheus.scrape", "loki.source", etc.
    // This is a basic heuristic check
    trimmed := strings.TrimSpace(contents)
    return len(trimmed) > 0 && !strings.HasPrefix(trimmed, "receivers:")
}

func looksLikeOTELConfig(contents string) bool {
    // OTEL configs are YAML with specific top-level keys
    var config map[string]interface{}
    if err := yaml.Unmarshal([]byte(contents), &config); err != nil {
        return false
    }

    // Must have service section at minimum
    _, hasService := config["service"]
    return hasService
}
```

## Common Issues and Solutions

### Pipeline not assigned to collectors

**Symptoms**: Pipeline created but collectors don't receive it

**Checks**:
1. Pipeline `enabled: true`?
2. Collector `enabled: true`?
3. Matcher syntax correct? (Prometheus format)
4. Collector attributes match pipeline matchers?
5. Collector polling? (default 5m poll_frequency)

**Debug**:
- Check collector logs for matcher evaluation
- Use Fleet Management UI Inventory view to see collector attributes
- Test matchers with ListCollectors API

### Configuration validation errors

**Symptoms**: Pipeline created but collectors show errors

**Causes**:
1. Invalid Alloy/OTEL configuration syntax
2. ConfigType mismatch (Alloy config with OpenTelemetryCollector type or vice versa)
3. Alloy config assigned to OTEL collector or vice versa

**Solution**:
- Use `validate_only: true` flag before actual creation
- Validate configType matches contents syntax
- Validate locally with `alloy fmt` (for Alloy) or `otelcol validate` (for OTEL)
- Check collector internal logs for specific errors
- Verify matchers assign pipeline to correct collector type
- Use Pipeline revision history to identify breaking change

**ConfigType mismatch example**:
```yaml
# WRONG: Alloy config but marked as OpenTelemetryCollector
spec:
  configType: OpenTelemetryCollector
  contents: |
    prometheus.scrape "default" { }  # This is Alloy syntax!

# CORRECT: Match configType to syntax
spec:
  configType: Alloy
  contents: |
    prometheus.scrape "default" { }
```

### Updates not applied

**Symptoms**: Update CRD but collectors don't see changes

**Checks**:
1. status.observedGeneration matches metadata.generation?
2. Controller logs show successful reconciliation?
3. Rate limits exceeded?
4. Collector poll interval not reached yet?

**Debug**:
- Check controller logs for API errors
- Verify status.updatedAt changed
- Check status conditions for errors
- Increase controller log level

### Duplicate pipelines

**Symptoms**: Multiple pipelines with same name

**Cause**: Name collision (shouldn't happen with proper controller)

**Solution**:
- Pipeline name must be unique across entire Fleet Management
- Consider namespace prefixing: `{namespace}-{name}`
- Use admission webhook to prevent duplicates
- Check for external pipeline creation (Terraform, UI)

### Deletion hangs

**Symptoms**: Pipeline CRD stuck in Terminating state

**Cause**: Finalizer not removed or DeletePipeline API call failing

**Debug**:
- Check controller logs for deletion errors
- Verify finalizer logic executes
- Test DeletePipeline API call manually
- Check for network issues or auth failures

**Resolution**:
- Fix controller deletion logic
- As last resort, manually remove finalizer: `kubectl patch pipeline <name> -p '{"metadata":{"finalizers":[]}}' --type=merge`

## Example Pipeline CRD

```yaml
apiVersion: fleetmanagement.grafana.com/v1alpha1
kind: Pipeline
metadata:
  name: prometheus-self-monitoring
  namespace: observability
spec:
  # Optional: if not set, uses metadata.name
  name: prometheus-self-monitoring

  # Alloy configuration
  contents: |
    prometheus.exporter.self "alloy" { }

    prometheus.scrape "alloy" {
      targets    = prometheus.exporter.self.alloy.targets
      forward_to = [prometheus.remote_write.grafanacloud.receiver]
      scrape_interval = "60s"
    }

    prometheus.remote_write "grafanacloud" {
      external_labels = {"collector_id" = constants.hostname}

      endpoint {
        url = env("PROMETHEUS_URL")

        basic_auth {
          username      = env("PROMETHEUS_USER")
          password_file = "/etc/secrets/prometheus-password"
        }
      }
    }

  # Assign to Linux collectors in production
  matchers:
    - collector.os=linux
    - environment=production

  # Enable the pipeline
  enabled: true

  # Configuration type (Alloy or OpenTelemetryCollector)
  configType: Alloy

  # Optional: track source
  source:
    type: Git
    namespace: main-branch
```

Example Pipeline with OpenTelemetry Collector configuration:

```yaml
apiVersion: fleetmanagement.grafana.com/v1alpha1
kind: Pipeline
metadata:
  name: otel-metrics-pipeline
  namespace: observability
spec:
  # OpenTelemetry Collector configuration
  contents: |
    receivers:
      otlp:
        protocols:
          grpc:
            endpoint: 0.0.0.0:4317
          http:
            endpoint: 0.0.0.0:4318

    processors:
      batch:
        timeout: 10s
        send_batch_size: 1024

    exporters:
      prometheusremotewrite:
        endpoint: ${env:PROMETHEUS_URL}
        auth:
          authenticator: basicauth

    extensions:
      basicauth:
        client_auth:
          username: ${env:PROMETHEUS_USER}
          password: ${env:PROMETHEUS_PASSWORD}

    service:
      extensions: [basicauth]
      pipelines:
        metrics:
          receivers: [otlp]
          processors: [batch]
          exporters: [prometheusremotewrite]

  # Assign to collectors running OTEL
  matchers:
    - collector.type=otel
    - environment=production

  enabled: true

  # Must specify OpenTelemetryCollector for OTEL config
  configType: OpenTelemetryCollector

  source:
    type: Git
    namespace: main-branch
```

Expected status after reconciliation:

```yaml
status:
  id: "12345"
  observedGeneration: 1
  createdAt: "2024-01-15T10:30:00Z"
  updatedAt: "2024-01-15T10:30:00Z"
  revisionId: "67890"
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: Pipeline successfully synced to Fleet Management
    lastTransitionTime: "2024-01-15T10:30:00Z"
  - type: Synced
    status: "True"
    reason: UpsertSucceeded
    message: UpsertPipeline API call succeeded
    lastTransitionTime: "2024-01-15T10:30:00Z"
```
