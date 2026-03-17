# Observability Implementation Guide: Concurrent GVK Processing

**Feature**: 003-concurrent-gvk-processing  
**Document**: Observability specifications and measurement strategies  
**Created**: 2026-03-17  
**Status**: Reference

---

## Table of Contents

1. [Per-GVK Timing Measurement](#per-gvk-timing-measurement)
2. [Structured Logging Strategy](#structured-logging-strategy)
3. [Sync Summary Reports](#sync-summary-reports)
4. [Prometheus Metrics](#prometheus-metrics-phase-2)
5. [Log Queries & Alerting](#log-queries--alerting)

---

## Per-GVK Timing Measurement

### Key Metrics to Track

For each GVK fetch operation, measure:

```go
type GVKFetchMetrics struct {
    // Identifiers
    GVK             string        // e.g., "cert-manager.io/v1/Certificate"
    Context         string        // K8s context name
    
    // Timing
    StartTime       time.Time     // When fetch goroutine started
    EndTime         time.Time     // When fetch completed
    Duration        time.Duration // EndTime - StartTime
    
    // Resource counts
    ResourceCount   int           // Total resources fetched
    Pages           int           // Number of paginated list() calls
    PageSizes       []int         // Size of each page (optional)
    
    // Error & Retry info
    Status          string        // "success", "failed", "partial"
    ErrorMessage    string        // Error details if failed
    RetryCount      int           // Number of retries attempted
    LastErrorType   string        // e.g., "timeout", "rbac", "api_error"
    
    // Performance
    ThroughputPerSec float64      // resources/second = ResourceCount / Duration.Seconds()
    AvgPageSize     float64       // Average resources per page
}
```

### Measurement Implementation

**Step 1: Start Timer**
```go
g.Go(func() error {
    metrics := GVKFetchMetrics{
        GVK: crSpec.GVK,
        Context: contextName,
        StartTime: time.Now(),
    }
    
    // Defer end-of-function cleanup
    defer func() {
        metrics.EndTime = time.Now()
        metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
        if metrics.ResourceCount > 0 {
            metrics.ThroughputPerSec = float64(metrics.ResourceCount) / metrics.Duration.Seconds()
            metrics.AvgPageSize = float64(metrics.ResourceCount) / float64(metrics.Pages)
        }
        c.recordMetrics(metrics)
    }()
    
    // ... fetch logic ...
})
```

**Step 2: Count Resources During Fetch**
```go
// In fetchGVKResources():
func (c *client) fetchGVKResources(ctx context.Context, gvk string, metrics *GVKFetchMetrics) error {
    var allResources []unstructured.Unstructured
    continueToken := ""
    
    for {
        list, err := c.listResources(ctx, gvk, continueToken)
        if err != nil {
            metrics.ErrorMessage = err.Error()
            metrics.Status = "failed"
            metrics.LastErrorType = classifyError(err)
            return err
        }
        
        metrics.Pages++
        pageSize := len(list.Items)
        metrics.PageSizes = append(metrics.PageSizes, pageSize)
        metrics.ResourceCount += pageSize
        
        allResources = append(allResources, list.Items...)
        
        if list.Continue == "" {
            break
        }
        continueToken = list.Continue
    }
    
    metrics.Status = "success"
    return nil
}
```

**Step 3: Track Retries**
```go
func (c *client) fetchWithRetry(ctx context.Context, gvk string, metrics *GVKFetchMetrics) error {
    attempt := 0
    var lastErr error
    
    for attempt < maxRetries {
        if err := c.fetchGVKResources(ctx, gvk, metrics); err != nil {
            attempt++
            metrics.RetryCount = attempt
            metrics.LastErrorType = classifyError(err)
            lastErr = err
            
            if !isTransient(err) {
                metrics.ErrorMessage = fmt.Sprintf("%s (non-transient, no more retries)", err.Error())
                return err
            }
            
            // Backoff and retry...
        } else {
            return nil // Success
        }
    }
    
    metrics.Status = "failed"
    metrics.ErrorMessage = fmt.Sprintf("max retries (%d) exceeded: %v", maxRetries, lastErr)
    return lastErr
}
```

### Timing Accuracy Considerations

1. **Don't include queue wait time**: Start timer after goroutine runs, not when submitted
2. **Use monotonic time**: `time.Now()` includes monotonic clock (immune to system clock adjustments)
3. **Measure API call duration only**: Don't include conversion or validation time
4. **Account for pagination**: Multiple list() calls per GVK should be included in total duration

---

## Structured Logging Strategy

### Log Fields Standard

Every log entry related to GVK processing should include:

```json
{
  "timestamp": "2026-03-17T14:23:45.123456Z",
  "level": "info",
  "component": "custom_resources",
  "gvk": "cert-manager.io/v1/Certificate",
  "context": "prod-cluster",
  "event": "gvk_fetch_completed",
  "duration_ms": 342,
  "resource_count": 150,
  "page_count": 3,
  "status": "success",
  "throughput_per_sec": 438.6
}
```

### Log Levels & When to Use

| Level | Event | Example |
|-------|-------|---------|
| **DEBUG** | Detailed pagination, each page fetched | `page=2, items=50, continue_token=...` |
| **INFO** | GVK started, GVK completed | `gvk_fetch_started`, `gvk_fetch_completed` |
| **WARN** | Transient error with retry | `gvk_fetch_retrying: timeout, attempt 1/3` |
| **WARN** | Permanent error, no retry | `gvk_fetch_failed: RBAC denied` |
| **ERROR** | Unexpected internal error | Should not happen with proper error handling |

### Logging Code Examples

**GVK Start**:
```go
c.logger.Info().
    Str("gvk", gvk).
    Str("context", contextName).
    Msg("GVK fetch started")
```

**GVK Page Fetched**:
```go
c.logger.Debug().
    Str("gvk", gvk).
    Int("page", pageNum).
    Int("items", len(items)).
    Str("continue_token", continueToken[:20]+"..."). // Truncate for readability
    Msg("Page fetched")
```

**GVK Complete (Success)**:
```go
c.logger.Info().
    Str("gvk", gvk).
    Str("status", "success").
    Dur("duration", duration).
    Int("resource_count", totalResources).
    Int("pages", pageCount).
    Float64("throughput_per_sec", throughput).
    Msg("GVK fetch completed")
```

**GVK Retry (Transient Error)**:
```go
c.logger.Warn().
    Str("gvk", gvk).
    Int("attempt", retryCount).
    Int("max_retries", maxRetries).
    Err(err).
    Str("error_type", "timeout").
    Dur("retry_backoff_ms", backoffDuration).
    Msg("GVK fetch failed, retrying...")
```

**GVK Failed (Permanent Error)**:
```go
c.logger.Warn().
    Str("gvk", gvk).
    Str("status", "failed").
    Err(err).
    Str("error_type", "rbac").
    Int("retries_attempted", retryCount).
    Msg("GVK fetch failed (permanent error, no more retries)")
```

---

## Sync Summary Reports

### Human-Readable Summary Output

Print at the end of every sync to stdout/stderr:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Custom Resources Sync Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Configuration: prod-cluster
Sync Period: 2026-03-17T14:22:00Z → 2026-03-17T14:22:01Z
Concurrency Level: 10

GVK Fetch Results:
┌────────────────────────────────┬──────────┬────────────┬────────────────┐
│ GVK                            │ Status   │ Duration   │ Resources (RPS)│
├────────────────────────────────┼──────────┼────────────┼────────────────┤
│ cert-manager.io/v1/Cert        │ ✓        │ 342ms      │ 150 (438/sec)  │
│ argoproj.io/v1alpha1/App       │ ✓        │ 521ms      │ 67 (128/sec)   │
│ istio.io/v1beta1/Gateway       │ ✓        │ 123ms      │ 25 (203/sec)   │
│ example.com/v1alpha1/Widget    │ ⚠ Failed │ 89ms       │ 0              │
│ prometheus.io/v1/AlertManager  │ ✓        │ 892ms      │ 234 (262/sec)  │
└────────────────────────────────┴──────────┴────────────┴────────────────┘

Aggregate Statistics:
  Attempted GVKs: 5
  Successful: 4 (80%)
  Failed: 1 (20%)
  
  Total Resources Synced: 476
  Total Duration: 982ms (concurrent)
  Sequential Estimate: 1967ms
  Speedup: 2.0x
  Average Throughput: 485 resources/sec
  Peak Workers Active: 4/10

Performance:
  Slowest GVK: prometheus.io/v1/AlertManager (892ms, 262/sec)
  Fastest GVK: istio.io/v1beta1/Gateway (123ms, 203/sec)
  Highest Throughput: cert-manager.io/v1/Certificate (438/sec)

Failed GVKs (action required):
  • example.com/v1alpha1/Widget
    Status: RBAC permission denied
    Error: "you do not have permission to list resources of kind Widget"
    Retries: 0 (permanent error)
    Action: Grant read permission for Widget CRD
    Hint: `kubectl create clusterrolebinding k8s-source-widget --clusterrole=system:aggregate-to-admin --serviceaccount=cloudquery:k8s-source`

Recommendations:
  ✓ Good: 80% success rate (1 GVK can be fixed with RBAC)
  ✓ Good: Speedup 2.0x with 10 concurrent workers
  ⚠ Note: prometheus.io/v1/AlertManager slowest (892ms) - consider pagination tuning
  ⚠ Note: Only 4/10 workers used - increase concurrency or add more GVKs

Next sync scheduled: 2026-03-17T14:27:00Z (in +5m)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### Summary Implementation

```go
type SyncSummary struct {
    Context             string
    StartTime           time.Time
    EndTime             time.Time
    TotalDuration       time.Duration
    EstimatedSequential time.Duration
    ConcurrencyLevel    int
    TotalGVKs           int
    SuccessfulGVKs      int
    FailedGVKs          int
    TotalResources      int
    AverageThroughput   float64
    MaxConcurrency      int
    SlowestGVK          string
    FastestGVK          string
    GVKMetrics          []GVKFetchMetrics
    FailedDetails       map[string]string
}

func (c *client) generateSyncSummary(metrics []GVKFetchMetrics) SyncSummary {
    summary := SyncSummary{
        TotalGVKs: len(metrics),
    }
    
    // Calculate aggregates
    maxDuration := time.Duration(0)
    totalDuration := time.Duration(0)
    
    for _, m := range metrics {
        totalDuration += m.Duration
        if m.Duration > maxDuration {
            maxDuration = m.Duration
            summary.SlowestGVK = m.GVK
        }
        
        if m.Status == "success" {
            summary.SuccessfulGVKs++
            summary.TotalResources += m.ResourceCount
            summary.AverageThroughput += m.ThroughputPerSec
        } else {
            summary.FailedGVKs++
        }
    }
    
    summary.EstimatedSequential = totalDuration
    summary.TotalDuration = maxDuration // Concurrent = slowest
    summary.AverageThroughput /= float64(summary.SuccessfulGVKs)
    
    return summary
}

func (c *client) printSyncSummary(summary SyncSummary) {
    // Use table formatting library (e.g., github.com/jedib0t/go-pretty/table)
    // Print each section with clear formatting
}
```

---

## Prometheus Metrics (Phase 2)

### Metric Definitions

```go
var (
    gvkFetchDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:        "k8s_custom_resources_gvk_fetch_duration_seconds",
            Help:        "Time to fetch custom resources for a specific GVK",
            Buckets:     []float64{.01, .05, .1, .5, 1, 2, 5, 10},
            NativeHistogramBucketFactor: 1.1,
        },
        []string{"gvk", "status", "context"},
    )
    
    gvkResourcesCount = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "k8s_custom_resources_gvk_resources_fetched",
            Help: "Number of resources fetched for a specific GVK",
        },
        []string{"gvk", "context"},
    )
    
    gvkFetchErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "k8s_custom_resources_gvk_fetch_errors_total",
            Help: "Total number of GVK fetch failures by error type",
        },
        []string{"gvk", "error_type", "context"},
    )
    
    gvkRetries = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "k8s_custom_resources_gvk_fetch_retries",
            Help:    "Number of retries per GVK fetch",
            Buckets: []float64{0, 1, 2, 3, 5, 10},
        },
        []string{"gvk", "context"},
    )
    
    gvkThroughput = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "k8s_custom_resources_gvk_throughput_per_sec",
            Help: "Resources fetched per second for a GVK",
        },
        []string{"gvk", "context"},
    )
    
    syncConcurrency = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "k8s_custom_resources_concurrent_fetches_active",
            Help: "Number of concurrent GVK fetches currently running",
        },
        []string{"context"},
    )
)
```

### Recording Metrics

```go
// In fetchGVKResources():
gvkFetchDuration.WithLabelValues(gvk, "success", context).Observe(metrics.Duration.Seconds())
gvkResourcesCount.WithLabelValues(gvk, context).Set(float64(metrics.ResourceCount))
gvkThroughput.WithLabelValues(gvk, context).Set(metrics.ThroughputPerSec)
gvkRetries.WithLabelValues(gvk, context).Observe(float64(metrics.RetryCount))

if err != nil {
    gvkFetchErrors.WithLabelValues(gvk, classifyError(err), context).Inc()
}
```

---

## Log Queries & Alerting

### Loki Query Examples

```promql
# Find all GVK fetch completions in last 5 minutes
{component="custom_resources", event="gvk_fetch_completed"} | json | line_format "{{.gvk}} {{.duration_ms}}ms {{.resource_count}} resources"

# Slow GVKs (>500ms)
{component="custom_resources", event="gvk_fetch_completed"} | json duration_ms | duration_ms > 500

# Failed GVKs with error type
{component="custom_resources", status="failed"} | json | line_format "{{.gvk}}: {{.error_type}}"

# Average throughput per GVK
{component="custom_resources", event="gvk_fetch_completed"} | json | stats avg(throughput_per_sec) by gvk
```

### Alert Rules (PrometheusAlertRule)

```yaml
groups:
  - name: custom_resources
    rules:
      - alert: GVKFetchSlowdown
        expr: histogram_quantile(0.95, k8s_custom_resources_gvk_fetch_duration_seconds) > 5
        for: 5m
        annotations:
          summary: "GVK fetch taking longer than 5 seconds (P95)"
      
      - alert: GVKFetchHighErrorRate
        expr: rate(k8s_custom_resources_gvk_fetch_errors_total[5m]) > 0.1
        for: 2m
        annotations:
          summary: "GVK fetch error rate > 10%"
      
      - alert: GVKFetchHighRetryCount
        expr: histogram_quantile(0.99, k8s_custom_resources_gvk_fetch_retries) >= 3
        for: 10m
        annotations:
          summary: "GVK fetch requiring 3+ retries"
      
      - alert: LowConcurrencyUtilization
        expr: k8s_custom_resources_concurrent_fetches_active < 2
        for: 5m
        annotations:
          summary: "Only {{$value}} concurrent workers active (expected 10+)"
```

---

## Testing Observability

### Unit Tests for Metrics

```go
func TestGVKMetricsCollection(t *testing.T) {
    metrics := GVKFetchMetrics{
        GVK: "test.io/v1/Test",
        StartTime: time.Now(),
        ResourceCount: 100,
    }
    
    // Simulate 500ms duration
    metrics.EndTime = metrics.StartTime.Add(500 * time.Millisecond)
    metrics.Duration = metrics.EndTime.Sub(metrics.StartTime)
    metrics.ThroughputPerSec = float64(metrics.ResourceCount) / metrics.Duration.Seconds()
    
    assert.Equal(t, 100, metrics.ResourceCount)
    assert.Equal(t, 200.0, metrics.ThroughputPerSec) // 100 / 0.5 = 200
}

func TestSyncSummaryCalculation(t *testing.T) {
    metrics := []GVKFetchMetrics{
        {GVK: "a", ResourceCount: 100, Duration: 1 * time.Second, Status: "success"},
        {GVK: "b", ResourceCount: 50, Duration: 2 * time.Second, Status: "success"},
        {GVK: "c", ResourceCount: 0, Duration: 0 * time.Second, Status: "failed"},
    }
    
    summary := generateSyncSummary(metrics)
    
    assert.Equal(t, 3, summary.TotalGVKs)
    assert.Equal(t, 2, summary.SuccessfulGVKs)
    assert.Equal(t, 1, summary.FailedGVKs)
    assert.Equal(t, 150, summary.TotalResources)
    assert.Equal(t, 2*time.Second, summary.TotalDuration) // max = 2s (concurrent)
}
```

---

## Observability Checklist

- [ ] **Timing**: Per-GVK start/end time captured
- [ ] **Resource Count**: Resources and pages tracked during fetch
- [ ] **Error Classification**: Error types categorized (rbac, timeout, api_error, etc.)
- [ ] **Retry Tracking**: Retry count recorded for each GVK
- [ ] **Structured Logs**: All logs include GVK, context, duration, status
- [ ] **Summary Report**: Human-readable summary printed at end of sync
- [ ] **Metrics**: GVKFetchMetrics type records all observability data
- [ ] **Tests**: Unit tests verify metrics collection accuracy
- [ ] **Alerts**: Prometheus rules alert on slow/failed GVKs
- [ ] **Dashboard**: Grafana dashboard shows GVK performance

---

**This document provides the complete blueprint for implementing observability in the concurrent GVK processing feature.**
