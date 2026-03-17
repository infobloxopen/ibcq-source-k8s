# Implementation Plan: Concurrent GVK Processing

**Feature**: 003-concurrent-gvk-processing  
**Total Effort**: ~12-16 hours across 4 phases  
**Target Branch**: `003-concurrent-gvk-processing`  
**Start Date**: 2026-03-17  
**Release**: v0.5.0  

## Phase Overview

| Phase | Name | Duration | Effort | Goal |
|-------|------|----------|--------|------|
| 1 | Concurrent Fetcher Core | 4-6 hrs | Medium | `fetchCustomResourcesParallel()` with errgroup + bounded concurrency |
| 2 | Error Resilience | 2-3 hrs | Low | Per-GVK error collection + partial success + logging |
| 3 | Rate Limiting & Backoff | 2-3 hrs | Low | Exponential backoff, transient error retry, API throttling |
| 4 | Testing & Validation | 3-4 hrs | Medium | Unit tests, benchmarks, E2E validation with real cluster |

---

## Phase 1: Concurrent Fetcher Core (4-6 hours)

### Objective
Implement parallel GVK processing using `golang.org/x/sync/errgroup` with bounded concurrency.

### Deliverables

#### 1.1 Create ConcurrencyConfig type
**File**: `ibcq-source-k8s/resources/services/customresources/config.go` (new)

```go
package customresources

// ConcurrencyConfig holds concurrency limits and timeouts
type ConcurrencyConfig struct {
    MaxConcurrentGVKs int           // Max parallel GVK fetches (default: 10)
    FetchTimeout      time.Duration // Timeout per GVK (default: 5m)
    RetryAttempts     int           // Retry count on transient errors (default: 3)
    RetryBackoff      time.Duration // Initial backoff (default: 100ms)
}

// DefaultConcurrencyConfig returns safe defaults for cluster size
func DefaultConcurrencyConfig() ConcurrencyConfig {
    return ConcurrencyConfig{
        MaxConcurrentGVKs: 10,
        FetchTimeout:      5 * time.Minute,
        RetryAttempts:     3,
        RetryBackoff:      100 * time.Millisecond,
    }
}
```

**Why**: Encapsulates concurrency parameters, allows future CLI flags.

#### 1.2 Create FetchResult type
**File**: `ibcq-source-k8s/resources/services/customresources/types.go` (new)

```go
package customresources

// FetchResult holds outcome of one GVK fetch
type FetchResult struct {
    GVK       string                    // e.g., "cert-manager.io/v1/Certificate"
    Resources []unstructured.Unstructured
    Error     error
    Duration  time.Duration
    Attempts  int
}
```

**Why**: Decouples fetch logic from result handling; enables observability logging.

#### 1.3 Refactor fetchCustomResources to concurrent
**File**: `ibcq-source-k8s/resources/services/customresources/custom_resources.go`

Replace current sequential loop with:

```go
func (c *customResourcesClient) fetchCustomResources(ctx context.Context, ...) ([][]sdk.CQTypes, error) {
    config := DefaultConcurrencyConfig()
    
    g, ctx := errgroup.WithContext(ctx)
    g.SetLimit(config.MaxConcurrentGVKs)
    
    results := make([]FetchResult, len(c.spec.CustomResources))
    mu := sync.Mutex{}
    
    for i, crSpec := range c.spec.CustomResources {
        i, crSpec := i, crSpec
        
        g.Go(func() error {
            // Fetch with timeout
            fetchCtx, cancel := context.WithTimeout(ctx, config.FetchTimeout)
            defer cancel()
            
            res, err := c.fetchGVKResources(fetchCtx, crSpec.GVK)
            
            mu.Lock()
            results[i] = FetchResult{
                GVK:       crSpec.GVK,
                Resources: res,
                Error:     err,
                Duration:  ...,
            }
            mu.Unlock()
            
            return nil // Don't fail entire fetch
        })
    }
    
    if err := g.Wait(); err != nil {
        return nil, fmt.Errorf("concurrent fetch: %w", err)
    }
    
    // Convert results to CloudQuery rows
    rows := [][]sdk.CQTypes{}
    for _, res := range results {
        if res.Error != nil {
            c.logger.Warn().Err(res.Error).Str("gvk", res.GVK).Msg("GVK fetch failed")
            continue
        }
        for _, resource := range res.Resources {
            row, _ := c.convertToCustomResourceRow(resource)
            rows = append(rows, row)
        }
    }
    
    return rows, nil
}

func (c *customResourcesClient) fetchGVKResources(ctx context.Context, gvk string) ([]unstructured.Unstructured, error) {
    // Existing single-GVK fetch logic
    // No changes needed here
}
```

**Why**: Minimal changes to existing code; clear separation of concerns.

#### 1.4 Update Client initialization
**File**: `ibcq-source-k8s/client/client.go`

Ensure `dynamicClient` is cached and reusable (already done, no changes needed).

**Why**: Dynamic client is thread-safe and reusable for concurrent requests.

### Validation (Phase 1)

- [ ] Concurrent fetch compiles without errors
- [ ] Existing single-GVK tests still pass
- [ ] New test: `TestConcurrentFetch_MultipleGVKs` with 5 GVKs, verify all complete
- [ ] New test: `TestConcurrentFetch_BoundedQueue` with concurrency=2, 10 GVKs
- [ ] Benchmark: `BenchmarkConcurrentVsSequential` shows 3-5x speedup

**Estimated Time**: 4-6 hours

---

## Phase 2: Error Resilience (2-3 hours)

### Objective
Implement per-GVK error tracking and partial success semantics.

### Deliverables

#### 2.1 Add error aggregation
**File**: `ibcq-source-k8s/resources/services/customresources/custom_resources.go`

```go
type GVKError struct {
    GVK   string
    Err   error
    Count int // Resource count when error occurred
}

func (c *customResourcesClient) fetchCustomResources(...) error {
    // ... concurrent fetch ...
    
    errors := []GVKError{}
    successCount := 0
    
    for _, res := range results {
        if res.Error != nil {
            errors = append(errors, GVKError{
                GVK: res.GVK,
                Err: res.Error,
            })
        } else {
            successCount += len(res.Resources)
        }
    }
    
    // Log aggregated errors
    if len(errors) > 0 {
        c.logger.Warn().
            Int("failed_gvks", len(errors)).
            Int("succeeded_resources", successCount).
            Msg("Partial success in custom resource fetch")
        
        for _, err := range errors {
            c.logger.Debug().Err(err.Err).Str("gvk", err.GVK).Msg("Failed GVK")
        }
    }
    
    // Return success if at least 1 GVK succeeded
    if successCount > 0 {
        return nil // Partial success is acceptable
    }
    
    return fmt.Errorf("all GVK fetches failed: %v", errors)
}
```

**Why**: Allows one GVK failure without blocking others; clear error visibility.

#### 2.2 Add per-GVK timing logs
**File**: `ibcq-source-k8s/resources/services/customresources/custom_resources.go`

```go
g.Go(func() error {
    start := time.Now()
    res, err := c.fetchGVKResources(fetchCtx, crSpec.GVK)
    duration := time.Since(start)
    
    status := "success"
    if err != nil {
        status = "failed"
    }
    
    c.logger.Info().
        Str("gvk", crSpec.GVK).
        Str("status", status).
        Dur("duration_ms", duration).
        Int("resource_count", len(res)).
        Msg("GVK fetch completed")
    
    // ... rest of logic
})
```

**Why**: Enables debugging of slow GVKs and concurrent behavior verification.

#### 2.3 Create metrics.go with observability types
**File**: `ibcq-source-k8s/resources/services/customresources/metrics.go` (new)

```go
package customresources

// GVKFetchMetrics holds timing and status info for one GVK fetch
type GVKFetchMetrics struct {
    GVK              string
    StartTime        time.Time
    EndTime          time.Time
    Duration         time.Duration
    ResourceCount    int
    Pages            int
    RetryCount       int
    Status           string // "success", "failed", "partial"
    ErrorMessage     string
    ThroughputPerSec float64
}

// SyncSummary aggregates metrics across all GVKs in sync
type SyncSummary struct {
    Context              string
    StartTime            time.Time
    EndTime              time.Time
    TotalDuration        time.Duration
    EstimatedSequential  time.Duration
    ConcurrencyLevel     int
    TotalGVKs            int
    SuccessfulGVKs       int
    FailedGVKs           int
    TotalResources       int
    AverageThroughput    float64
    GVKMetrics           []GVKFetchMetrics
    FailedDetails        map[string]string // GVK -> error reason
}

// RecordGVKMetric records timing for one GVK
func (c *customResourcesClient) RecordGVKMetric(metrics GVKFetchMetrics) {
    // Store in memory or export to Prometheus
}

// GenerateSyncSummary creates human-readable summary
func (c *customResourcesClient) GenerateSyncSummary(
    context string,
    startTime time.Time,
    metrics []GVKFetchMetrics,
) SyncSummary {
    // Calculate aggregate metrics
    // Sort by performance
    // Format human-readable output
}
```

**Why**: Centralized metrics collection, reusable across logging and Prometheus exports.

#### 2.4 Add summary logging at end of sync
**File**: `ibcq-source-k8s/resources/services/customresources/custom_resources.go` (update)

```go
func (c *customResourcesClient) fetchCustomResources(...) error {
    // ... concurrent fetch logic ...
    
    summary := c.GenerateSyncSummary(ctx.Value("context"), startTime, allMetrics)
    
    c.logger.Info().
        Str("context", summary.Context).
        Int("total_gvks", summary.TotalGVKs).
        Int("successful", summary.SuccessfulGVKs).
        Int("failed", summary.FailedGVKs).
        Int("total_resources", summary.TotalResources).
        Dur("duration", summary.TotalDuration).
        Float64("speedup", summary.EstimatedSequential.Seconds() / summary.TotalDuration.Seconds()).
        Float64("throughput", summary.AverageThroughput).
        Msg("Custom resources sync completed")
    
    // Print human-readable summary to stdout
    c.printSyncSummary(summary)
    
    return nil
}

func (c *customResourcesClient) printSyncSummary(summary SyncSummary) {
    // Print table with GVK results
    // Print aggregate stats
    // Print failed GVK details
}
```

**Why**: Clear visibility into sync performance and failures.

### Validation (Phase 2)

- [ ] New test: `TestConcurrentFetch_PartialFailure` with 1 failed GVK out of 3
- [ ] New test: `TestErrorAggregation` verifies all errors logged
- [ ] New test: `TestMetricsCollection` verifies timing data is collected
- [ ] Logs show per-GVK timing and status
- [ ] Sync summary report is generated and printed
- [ ] Summary shows correct speedup calculation
- [ ] Sync continues even with one failed GVK

**Estimated Time**: 2-3 hours

---

## Phase 3: Rate Limiting & Backoff (2-3 hours)

### Objective
Implement exponential backoff for transient errors and API throttling protection.

### Deliverables

#### 3.1 Add exponential backoff retry
**File**: `ibcq-source-k8s/resources/services/customresources/retry.go` (new)

```go
package customresources

// IsTransientError checks if error is likely transient
func IsTransientError(err error) bool {
    if err == nil {
        return false
    }
    
    // Check for timeout or connection errors
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }
    
    if errors.Is(err, context.Canceled) {
        return false
    }
    
    // Check for API rate limit (429)
    if apierrors.IsStatusError(err) {
        code := apierrors.APIStatus(err).Status().Code
        return code == 429 || code == 503 || code == 504
    }
    
    return false
}

// WithRetry wraps a fetch operation with exponential backoff
func WithRetry(ctx context.Context, config ConcurrencyConfig, fn func() error) error {
    var lastErr error
    backoff := config.RetryBackoff
    
    for attempt := 0; attempt < config.RetryAttempts; attempt++ {
        if err := fn(); err == nil {
            return nil
        } else if IsTransientError(err) {
            lastErr = err
            if attempt < config.RetryAttempts-1 {
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case <-time.After(backoff):
                }
                backoff *= 2
                if backoff > 10*time.Second {
                    backoff = 10 * time.Second
                }
            }
        } else {
            return err // Non-transient error, don't retry
        }
    }
    
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

**Why**: Transient errors (timeout, 429) should retry; permanent errors (RBAC) should not.

#### 3.2 Integrate backoff into concurrent fetch
**File**: `ibcq-source-k8s/resources/services/customresources/custom_resources.go`

```go
g.Go(func() error {
    res, err := WithRetry(fetchCtx, config, func() error {
        return c.fetchGVKResources(fetchCtx, crSpec.GVK)
    })
    
    // ... log and store result
})
```

**Why**: Minimal change; leverages existing fetch logic.

#### 3.3 Dynamic concurrency tuning (Optional)
**File**: `ibcq-source-k8s/resources/services/customresources/config.go`

```go
// AdjustConcurrencyForCluster scales concurrency based on cluster size
func AdjustConcurrencyForCluster(nodeCount int) int {
    // Scale: 1 worker per 50 nodes, min 5, max 50
    workers := nodeCount / 50
    if workers < 5 {
        workers = 5
    }
    if workers > 50 {
        workers = 50
    }
    return workers
}
```

**Why**: Avoids overwhelming small clusters; maximizes throughput on large clusters.

### Validation (Phase 3)

- [ ] New test: `TestRetry_TransientError` simulates timeout, verifies retry succeeds
- [ ] New test: `TestRetry_PermanentError` simulates RBAC error, no retry
- [ ] New test: `TestBackoffJitter` verifies exponential backoff timing
- [ ] Benchmark: Concurrent fetch with 1 transient error recovers
- [ ] Logs show retry attempts

**Estimated Time**: 2-3 hours

---

## Phase 4: Testing & Validation (3-4 hours)

### Objective
Comprehensive testing to prove concurrent behavior and prevent regressions.

### Deliverables

#### 4.1 Unit Tests
**File**: `ibcq-source-k8s/resources/services/customresources/custom_resources_test.go` (update)

```go
func TestConcurrentFetch_MultipleGVKs(t *testing.T) {
    // 5 GVKs, verify all complete
    // Assertions:
    // - All 5 results returned
    // - No error
    // - All resources present
}

func TestConcurrentFetch_BoundedQueue(t *testing.T) {
    // 10 GVKs, concurrency=2, measure max parallel
    // Assertions:
    // - Never more than 2 goroutines running simultaneously
    // - All 10 complete
}

func TestConcurrentFetch_PartialFailure(t *testing.T) {
    // 3 GVKs, 1 fails RBAC
    // Assertions:
    // - Other 2 succeed
    // - Error logged but not returned
    // - Partial success (2 GVKs) acceptable
}

func TestErrorAggregation(t *testing.T) {
    // Multiple errors across GVKs
    // Assertions:
    // - All errors collected
    // - All errors logged
    // - Sync completes with partial results
}

func TestRetry_TransientError(t *testing.T) {
    // Mock timeout on first attempt, success on second
    // Assertions:
    // - Retry triggered
    // - Second attempt succeeds
    // - Backoff delay observed
}

func TestConcurrentMemoryOverhead(t *testing.T) {
    // Measure memory before/after concurrent fetch
    // Assertions:
    // - Memory overhead ≤20%
}
```

**Why**: Validates concurrent behavior, prevents regressions.

#### 4.2 Benchmarks
**File**: `ibcq-source-k8s/resources/services/customresources/benchmark_test.go` (new)

```go
func BenchmarkConcurrentVsSequential(b *testing.B) {
    // 10 GVKs × 100 resources each
    // Concurrent: Should be ~5-10x faster than sequential
    // Assertions:
    // - Concurrent < 1.5x single GVK time
    // - Sequential ≈ 10x single GVK time
}

func BenchmarkConcurrentScaling(b *testing.B) {
    // Vary GVK count (1, 5, 10, 50)
    // Assertions:
    // - Linear scaling until concurrency limit
    // - Flat after concurrency limit
}

func BenchmarkBackoffOverhead(b *testing.B) {
    // Retry with backoff vs no retry
    // Assertions:
    // - Overhead negligible if no errors (<5%)
    // - Proportional to retry count if errors
}
```

**Why**: Proves performance improvement; detects regressions.

#### 4.3 Integration Tests (Real K8s Cluster)
**File**: `ibcq-source-k8s/resources/services/customresources/integration_test.go` (new)

```go
func TestIntegration_ConcurrentCustomResourceSync(t *testing.T) {
    // Setup: 5 GVK configs pointing to real CRs
    // Action: Fetch all concurrently
    // Assertions:
    // - All 5 GVKs complete
    // - All resources retrieved
    // - Timing shows concurrency (not sum)
    // - All resources stored in destination
}

func TestIntegration_PartialFailure(t *testing.T) {
    // Setup: 3 GVKs, RBAC deny for one
    // Action: Concurrent fetch
    // Assertions:
    // - 2 GVKs complete
    // - Error logged for 1 GVK
    // - Partial sync succeeds
}
```

**Why**: Validates behavior with real K8s API and CloudQuery destination.

#### 4.4 E2E Validation Script
**File**: `ibcq-source-k8s/test/e2e_concurrent_test.sh` (new)

```bash
#!/bin/bash

# E2E test script for concurrent GVK processing

# Setup:
# 1. Create minikube cluster with 5 custom resources
# 2. Configure config.yml with all 5 GVKs
# 3. Run sync
# 4. Verify:
#    - All 5 GVKs fetched
#    - Resources in PostgreSQL
#    - Timing shows concurrency
#    - Logs show per-GVK status

# Example output:
# ✅ GVK cert-manager.io/v1/Certificate: 100 resources, 0.5s
# ✅ GVK argoproj.io/v1alpha1/Application: 50 resources, 0.7s
# ✅ GVK istio.io/v1beta1/Gateway: 25 resources, 0.3s
# ✅ GVK example.com/v1alpha1/Widget: 10 resources, 0.2s
# ⏱️  Total: 0.8s (concurrent) vs 1.7s (sequential)
```

**Why**: Proves end-to-end functionality with real cluster and database.

### Validation (Phase 4)

- [ ] All 19 existing tests pass (no regression)
- [ ] 12+ new unit tests pass (concurrent behavior)
- [ ] 3+ new benchmarks show 5-10x improvement
- [ ] 2+ integration tests pass (real cluster)
- [ ] E2E script completes successfully
- [ ] Code coverage ≥85% for concurrent_resources.go

**Estimated Time**: 3-4 hours

---

## Implementation Checklist

### Phase 1 Setup
- [ ] Create `specs/003-concurrent-gvk/spec.md` (feature spec)
- [ ] Create `specs/003-concurrent-gvk/plan.md` (this file)
- [ ] Create feature branch: `git checkout -b 003-concurrent-gvk-processing`
- [ ] Update CHANGELOG.md with Phase 1 target

### Phase 1 Coding
- [ ] Create `config.go` with ConcurrencyConfig
- [ ] Create `types.go` with FetchResult
- [ ] Refactor `fetchCustomResources()` to concurrent version
- [ ] Verify dynamic client reusability
- [ ] Run `go test ./...` → all existing tests pass
- [ ] Commit: "feat(concurrent): Add concurrent GVK fetching with errgroup"

### Phase 1 Validation
- [ ] Write `TestConcurrentFetch_MultipleGVKs`
- [ ] Write `TestConcurrentFetch_BoundedQueue`
- [ ] Run tests → pass
- [ ] Write `BenchmarkConcurrentVsSequential`
- [ ] Benchmark shows 3-5x improvement
- [ ] Commit: "test(concurrent): Add concurrency tests and benchmarks"

### Phase 2 Coding
- [ ] Add GVKError type
- [ ] Refactor error handling to collect per-GVK errors
- [ ] Add per-GVK timing logs
- [ ] Commit: "feat(concurrent): Add error aggregation and observability"

### Phase 2 Validation
- [ ] Write `TestConcurrentFetch_PartialFailure`
- [ ] Write `TestErrorAggregation`
- [ ] Run tests → pass
- [ ] Manual test: one GVK fails, others succeed
- [ ] Commit: "test(concurrent): Add partial failure tests"

### Phase 3 Coding
- [ ] Create `retry.go` with IsTransientError and WithRetry
- [ ] Integrate retry into concurrent fetch
- [ ] Optional: Add dynamic concurrency tuning
- [ ] Commit: "feat(concurrent): Add exponential backoff retry"

### Phase 3 Validation
- [ ] Write `TestRetry_TransientError`
- [ ] Write `TestRetry_PermanentError`
- [ ] Run tests → pass
- [ ] Manual test: simulate timeout, verify retry succeeds
- [ ] Commit: "test(concurrent): Add retry tests"

### Phase 4 Coding
- [ ] Create `benchmark_test.go` with scaling benchmarks
- [ ] Create `integration_test.go` for real cluster tests
- [ ] Create `e2e_concurrent_test.sh` validation script
- [ ] Commit: "test(concurrent): Add integration and E2E tests"

### Phase 4 Validation
- [ ] Run full test suite: `go test ./...`
- [ ] All 19 existing tests pass
- [ ] 12+ new tests pass
- [ ] Run benchmarks: `go test -bench=. -benchtime=3s`
- [ ] Concurrent speedup ≥3x
- [ ] Run integration tests on real cluster
- [ ] E2E script completes successfully
- [ ] Final commit: "test(concurrent): All phase 4 validation complete"

### Final Tasks
- [ ] Update `README.md` with concurrency section
- [ ] Update `CHANGELOG.md` with v0.5.0 notes
- [ ] Update documentation: `docs/_configuration.md` with concurrency options
- [ ] Create PR with squashed commits
- [ ] PR description: Links to spec, lists all validation results
- [ ] Code review and merge

---

## Key Milestones

| Milestone | Criteria | Target |
|-----------|----------|--------|
| Phase 1 Complete | Concurrent fetch working, tests passing | Day 1 (4-6 hrs) |
| Phase 2 Complete | Error resilience + observability | Day 1-2 (2-3 hrs) |
| Phase 3 Complete | Retry + backoff implemented | Day 2 (2-3 hrs) |
| Phase 4 Complete | All tests passing, benchmarks validated | Day 2-3 (3-4 hrs) |
| PR Ready | All validation complete, docs updated | Day 3 |

---

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Concurrent goroutines exhaust memory | Medium | BoundedSemaphore + benchmarks verify ≤20% overhead |
| One GVK hangs, blocks sync | High | Context timeout per GVK (5m default) |
| Error logging overwhelms logs | Low | Per-GVK errors logged at WARN, not ERROR |
| API rate limiting triggered | Medium | Exponential backoff + jitter |
| Tests flaky due to timing | Low | Deterministic mocks, avoid timing assertions |

---

## Success Definition

**Implementation Complete When**:
1. ✅ All 4 phases delivered
2. ✅ 31 tests pass (19 existing + 12+ new)
3. ✅ Concurrent speedup ≥3x for 5+ GVKs
4. ✅ Partial failure handling proven
5. ✅ E2E validation with real cluster successful
6. ✅ Code review approved
7. ✅ PR merged to main branch
8. ✅ Documentation updated for v0.5.0

---

## Dependencies

- `golang.org/x/sync` (errgroup) — already available
- K8s client libraries — already in go.mod
- Test mocking — existing testing.go
- PostgreSQL test database — already in test environment
- Real K8s cluster (minikube) — already available

**No new external dependencies required.**
