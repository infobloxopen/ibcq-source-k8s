# Feature Specification: Concurrent GVK Processing

**Feature Branch**: `003-concurrent-gvk-processing`  
**Created**: 2026-03-17  
**Status**: Draft  
**Input**: Architecture weakness: "Sequential GVK processing - No concurrency between different GVKs"

## Summary

Implement concurrent processing of multiple Custom Resource GVK entries to reduce sync time from O(n) sequential to O(1) parallel. With proper error handling, partial failures won't block other GVKs. Target: **5-10x faster syncs** for multi-GVK configs.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Faster multi-GVK sync (Priority: P1)

As a platform engineer, I want concurrent GVK processing so my sync completes in seconds instead of minutes when collecting from multiple custom resource types.

**Why this priority**: Sequential processing is the primary performance bottleneck; concurrency is the highest-ROI optimization.

**Independent Test**: Measure sync time for 10 GVKs with concurrent vs sequential; verify 5-10x improvement.

**Acceptance Scenarios**:

1. **Given** 10 GVKs with similar resource counts, **When** sync runs, **Then** time ≈ time_for_single_GVK (not 10x).
2. **Given** mixed GVK latencies, **When** sync runs, **Then** completion time ≈ slowest_GVK (not sum).
3. **Given** error in one GVK, **When** sync runs, **Then** other GVKs complete successfully (partial success).

---

### User Story 2 - Partial failure resilience (Priority: P1)

As a user, I want one failed GVK to not block collection of successful GVKs, so I get partial results instead of complete failure.

**Why this priority**: Current fail-fast stops everything; partial success is better UX.

**Independent Test**: Sync with 3 GVKs where 1 fails RBAC; verify other 2 complete and are stored.

**Acceptance Scenarios**:

1. **Given** permission denied for cert-manager, **When** sync runs, **Then** ArgoCD and Istio CRs still sync.
2. **Given** transient API error for one GVK, **When** sync retries, **Then** retry succeeds (if transient).
3. **Given** GVK validation error, **When** sync completes, **Then** error is logged and sync continues.

---

### User Story 3 - Configurable concurrency (Priority: P2)

As a cluster operator, I want to tune concurrency level to avoid overwhelming the API server.

**Why this priority**: Protects against rate limits; secondary to core feature.

**Independent Test**: Run with `--max-concurrent=5` and verify queue limits.

**Acceptance Scenarios**:

1. **Given** `concurrency: 5` in spec, **When** 20 GVKs are processed, **Then** max 5 GVKs fetch simultaneously.
2. **Given** `concurrency: 0` (unlimited), **When** sync runs, **Then** all GVKs start at once.
3. **Given** concurrency mismatch, **When** config loads, **Then** warning is logged but sync proceeds.

---

### User Story 4 - Observability (Priority: P2)

As an operator, I want metrics and logs showing which GVKs completed, failed, and how long each took.

**Why this priority**: Debugging concurrent behavior requires visibility.

**Independent Test**: Run sync, check logs for per-GVK timing and status.

**Acceptance Scenarios**:

1. **Given** 5 GVK sync, **When** sync completes, **Then** log shows: `gvk=X duration=Y status=Z` for each.
2. **Given** partial failure, **When** sync ends, **Then** summary shows: "3 succeeded, 1 failed, 1 skipped".
3. **Given** concurrent execution, **When** logs are reviewed, **Then** timestamps show overlapping GVK processing.

### Edge Cases

- One GVK returns 100,000 resources → memory impact of parallelism.
- Mixed response times (one GVK 50ms, another 5000ms) → ensure slow GVK doesn't halt others.
- Network partition mid-sync → retries and partial recovery.
- API rate limiting hit during concurrent requests → backoff and jitter.
- Destination write bottleneck → concurrent discovery but serialized writes.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST fetch multiple GVKs concurrently using worker pool or errgroup.
- **FR-002**: One GVK fetch failure MUST NOT block other GVKs (partial success).
- **FR-003**: System MUST enforce maximum concurrent GVK fetches (configurable).
- **FR-004**: System MUST retry transient errors per GVK (exponential backoff).
- **FR-005**: System MUST emit per-GVK timing and status in logs.
- **FR-006**: System MUST handle resource limits gracefully (memory, API connections).
- **FR-007**: System MUST preserve write order (resources written sequentially to avoid race conditions in destination).
- **FR-008**: Total sync time MUST be ≤ max(single_GVK_time) + overhead (not sum).

### Technical Requirements

- **TR-001**: Use `golang.org/x/sync/errgroup` or equivalent for concurrency.
- **TR-002**: Implement per-GVK error collection (slice of errors, not early return).
- **TR-003**: Add context cancellation support (stop all goroutines on parent timeout).
- **TR-004**: Use semaphore or bounded queue to limit concurrent fetches.
- **TR-005**: Implement exponential backoff for transient failures (3 retries, 100ms-10s).

### Key Entities *(include if feature involves data)*

- **FetchJob**: Encapsulates one GVK fetch with status, result, error, timing.
- **FetchResult**: Contains resources, error, duration, retry count for one GVK.
- **ConcurrencyConfig**: Limits and backoff parameters (max_concurrent, timeout, retry_policy).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Single GVK sync time unchanged (no regression, ±5%).
- **SC-002**: 10-GVK sync time ≤ 1.5x slowest single GVK (5-10x improvement vs sequential).
- **SC-003**: 1000-resource GVK memory overhead ≤ 20% increase (from parallelism).
- **SC-004**: Partial failure rate ≤ 5% (transient errors caught by retry).
- **SC-005**: Log observability shows per-GVK duration, status, error for all GVKs.
- **SC-006**: All existing unit/integration tests pass (no regression).
- **SC-007**: New concurrency tests prove goroutine overlap (concurrent execution verified).

## Example Behavior

### Before (Sequential):
```
GVK 1 [████████] 1.0s ─┐
GVK 2              [████████] 1.0s ─┐
GVK 3                          [████████] 1.0s
Total: 3.0s
```

### After (Concurrent):
```
GVK 1 [████████] 1.0s ─┐
GVK 2 [████████] 1.0s ─├─ 1.2s (parallel)
GVK 3 [████████] 1.0s ─┘
Total: 1.2s (5x improvement)
```

### Partial Failure Example:
```
GVK 1 [████████] 1.0s ✅ 100 resources
GVK 2 [XXXXXX]   0.2s ❌ Permission denied → logged, continues
GVK 3 [████████] 1.0s ✅ 50 resources
Result: 150 resources synced, 1 error logged, sync completes
```

## Architecture Changes

### Current (Sequential):
```
fetchCustomResources()
  └─ for each crSpec in spec.CustomResources
      └─ fetchResourcesByGVR(crSpec.GVK)
          └─ if error: return immediately (fail-fast)
```

### Proposed (Concurrent):
```
fetchCustomResources()
  ├─ Create errgroup with cancellation
  ├─ Spawn goroutine per GVK
  │   ├─ Fetch resources (with retry)
  │   ├─ Collect results/errors (don't fail immediately)
  │   └─ Log per-GVK status + timing
  ├─ Wait for all goroutines
  ├─ Aggregate errors (non-fatal warnings)
  └─ Return success if ≥1 GVK succeeded
```

### Code Sketch:
```go
func fetchCustomResources(ctx context.Context, ...) error {
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(spec.Concurrency) // Bounded concurrency
    
    results := make([]FetchResult, len(spec.CustomResources))
    
    for i, crSpec := range spec.CustomResources {
        i, crSpec := i, crSpec // Capture loop vars
        g.Go(func() error {
            res, err := fetchWithRetry(ctx, crSpec.GVK, ...)
            results[i] = FetchResult{
                GVK: crSpec.GVK,
                Resources: res,
                Error: err,
                Duration: ...,
            }
            if err != nil {
                logger.Warn().Err(err).Str("gvk", crSpec.GVK).Msg("GVK fetch failed")
                return nil // Don't fail everything
            }
            return nil
        })
    }
    
    if err := g.Wait(); err != nil {
        return fmt.Errorf("concurrent fetch error: %w", err)
    }
    
    // Process results, emit combined rows to destination
    for _, res := range results {
        if res.Error != nil {
            continue // Skip failed GVKs
        }
        for _, resource := range res.Resources {
            row, _ := convertToCustomResourceRow(resource)
            res <- row // Send to CloudQuery
        }
    }
    
    return nil
}
```

## Out of Scope (Initial Version)

- Rate limiting per API server (use k8s native QPS limits)
- Caching of resources between syncs (future: watch API)
- Streaming writes (CloudQuery destination protocol)
- Batch size tuning per GVK (all use same batch)

## Configuration

```yaml
spec:
  concurrency: 10  # Max concurrent GVK fetches (default: 10)
  
custom_resources:
  - gvk: "cert-manager.io/v1/Certificate"
    # Individual GVK config, inherits concurrency from spec
```

## Observability & Metrics *(mandatory)*

### Per-GVK Timing Measurement

**How We Measure**:

```go
type GVKFetchMetrics struct {
    GVK              string        // e.g., "cert-manager.io/v1/Certificate"
    StartTime        time.Time     // When fetch started
    EndTime          time.Time     // When fetch completed
    Duration         time.Duration // Total elapsed time
    ResourceCount    int           // Number of resources fetched
    Pages            int           // Number of paginated requests
    RetryCount       int           // Number of retries attempted
    Status           string        // "success", "failed", "partial"
    ErrorMessage     string        // Error details if failed
    ThroughputPerSec float64       // resources/second
}
```

**Measurement Points**:

1. **Start**: `startTime := time.Now()` before `g.Go()`
2. **During**: Track page count, resource count as paginated
3. **End**: `endTime := time.Now()` after fetch completes
4. **Calculate**: `duration := endTime.Sub(startTime)`

**Example Code**:
```go
g.Go(func() error {
    metrics := GVKFetchMetrics{GVK: crSpec.GVK, StartTime: time.Now()}
    defer func() {
        metrics.EndTime = time.Now()
        metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
        metrics.ThroughputPerSec = float64(metrics.ResourceCount) / metrics.Duration.Seconds()
        recordMetrics(metrics)
    }()
    
    resources, err := fetchGVKResources(ctx, crSpec.GVK)
    metrics.ResourceCount = len(resources)
    
    if err != nil {
        metrics.Status = "failed"
        metrics.ErrorMessage = err.Error()
        metrics.RetryCount = getRetryCount(err)
    } else {
        metrics.Status = "success"
    }
    
    return nil
})
```

### Structured Logging Strategy

**Log Levels**:
- **DEBUG**: Per-page fetch details, retry attempts, pagination info
- **INFO**: GVK start, GVK completion with timing
- **WARN**: GVK fetch failed, transient errors encountered
- **ERROR**: Unexpected errors (should not occur with retry logic)

**Log Schema** (JSON structured):
```json
{
  "timestamp": "2026-03-17T14:23:45.123Z",
  "level": "info",
  "component": "custom_resources",
  "gvk": "cert-manager.io/v1/Certificate",
  "event": "gvk_fetch_started",
  "context": "prod-cluster",
  "concurrency_limit": 10,
  "queue_position": 3
}
```

**Key Fields Per Log**:
- `timestamp`: RFC3339 format
- `gvk`: Group/Version/Kind
- `context`: K8s context name
- `duration_ms`: Milliseconds elapsed (on completion)
- `resource_count`: Number of resources fetched
- `status`: "started", "completed", "failed", "retrying"
- `error`: Error message if applicable
- `retry_count`: Number of retries (if transient error)
- `throughput`: Resources per second

**Log Examples**:

```
[INFO] GVK fetch started
  gvk=cert-manager.io/v1/Certificate
  context=prod
  expected_resources=~100

[DEBUG] Page fetched
  gvk=cert-manager.io/v1/Certificate
  page=2
  items=50
  continue_token=abc123def456

[INFO] GVK fetch completed
  gvk=cert-manager.io/v1/Certificate
  status=success
  duration_ms=342
  resource_count=150
  pages=3
  throughput=438.6/sec

[WARN] GVK fetch failed (retrying)
  gvk=example.com/v1/Widget
  attempt=1
  error="context deadline exceeded"
  retry_in_ms=100
  max_retries=3

[WARN] GVK fetch failed (permanent)
  gvk=old-api.io/v1alpha1/Deprecated
  status=failed
  error="resource type not found"
  retries_attempted=0
  reason=PERMANENT_ERROR
```

### Metrics Collection & Export

**Prometheus Metrics** (Phase 2):
```go
var (
    gvkFetchDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "k8s_gvk_fetch_duration_seconds",
            Help:    "Time to fetch GVK resources",
            Buckets: []float64{.01, .05, .1, .5, 1, 2, 5, 10},
        },
        []string{"gvk", "status", "context"},
    )
    
    gvkResourcesCount = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "k8s_gvk_resources_fetched",
            Help: "Number of resources fetched per GVK",
        },
        []string{"gvk", "context"},
    )
    
    gvkFetchErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "k8s_gvk_fetch_errors_total",
            Help: "Total GVK fetch failures",
        },
        []string{"gvk", "error_type", "context"},
    )
    
    gvkRetries = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "k8s_gvk_fetch_retries",
            Help:    "Retry count per GVK",
            Buckets: []float64{0, 1, 2, 3, 5, 10},
        },
        []string{"gvk"},
    )
)
```

### Sync Summary Report (Human-Readable Output)

**End-of-Sync Summary**:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Custom Resources Sync Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Configuration: prod-cluster
Concurrency Level: 10
Sync Started: 2026-03-17T14:22:00Z
Sync Completed: 2026-03-17T14:22:01Z

GVK Results:
┌────────────────────────────┬──────────┬────────────┬────────────┐
│ GVK                        │ Status   │ Duration   │ Resources  │
├────────────────────────────┼──────────┼────────────┼────────────┤
│ cert-manager.io/v1/Cert... │ ✓ OK     │ 342ms      │ 150 (438/s)│
│ argoproj.io/v1alpha1/App   │ ✓ OK     │ 521ms      │ 67 (128/s) │
│ istio.io/v1beta1/Gateway   │ ✓ OK     │ 123ms      │ 25 (203/s) │
│ example.com/v1alpha1/Wid   │ ⚠ FAIL   │ 89ms       │ 0 (RBAC)   │
│ prometheus.io/v1/Alert     │ ✓ OK     │ 892ms      │ 234 (262/s)│
└────────────────────────────┴──────────┴────────────┴────────────┘

Aggregate Results:
  Total GVKs: 5
  Successful: 4 (80%)
  Failed: 1 (20%)
  Total Resources Synced: 476
  Total Duration: 982ms (concurrent)
  Sequential Duration: 1967ms (estimated)
  Speedup: 2.0x
  Average Throughput: 485 resources/sec
  Peak Concurrency: 4/10 workers active

Top Performers (by throughput):
  1. prometheus.io/v1/Alert: 262 res/sec (892ms, 234 resources)
  2. cert-manager.io/v1/Certificate: 438 res/sec (342ms, 150 resources)
  3. istio.io/v1beta1/Gateway: 203 res/sec (123ms, 25 resources)

Failed GVKs (requires action):
  • example.com/v1alpha1/Widget
    Status: FAILED
    Error: permission denied (RBAC)
    Attempts: 3 retries (permanent error, no more retries)
    Action: Grant read permission for Widget CRD

Next sync scheduled: 2026-03-17T14:27:00Z (in +5m)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

## Testing Strategy

1. **Unit tests**: 
   - Concurrent fetch with 5 GVKs, verify all complete
   - Partial failure (1 GVK fails), others succeed
   - Bounded queue (concurrency=2), 10 GVKs, verify max 2 parallel
   - Timing assertions: verify no synchronization overhead

2. **Integration tests**:
   - Real cluster, 10 custom resources, measure timing
   - Transient error simulation (mock timeout), verify retry succeeds
   - Verify logs contain per-GVK timing and resource counts

3. **Benchmarks**:
   - `BenchmarkConcurrentVsSequential`: 10 GVKs, measure speedup
   - `BenchmarkMemoryOverhead`: Verify ≤20% increase
   - `BenchmarkThroughput`: Resources per second per GVK

4. **E2E**:
   - Full sync with 5 GVKs, verify all resources stored
   - Partial failure scenario, verify partial sync
   - Verify sync summary report is generated
   - Verify logs contain all required observability fields
