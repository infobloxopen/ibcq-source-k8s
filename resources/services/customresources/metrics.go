package customresources

import (
	"fmt"
	"sort"
	"time"
)

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
	GVKMetrics          []GVKFetchMetrics
	FailedDetails       map[string]string // GVK -> error reason
}

// NewGVKFetchMetrics creates a new metrics object for a GVK fetch
func NewGVKFetchMetrics(gvk string) *GVKFetchMetrics {
	return &GVKFetchMetrics{
		GVK:        gvk,
		StartTime:  time.Now(),
		Pages:      0,
		RetryCount: 0,
	}
}

// Complete marks the fetch as complete and calculates metrics
func (m *GVKFetchMetrics) Complete(resourceCount int, err error) {
	m.EndTime = time.Now()
	m.Duration = m.EndTime.Sub(m.StartTime)
	m.ResourceCount = resourceCount

	if err != nil {
		m.Status = "failed"
		m.ErrorMessage = err.Error()
	} else {
		m.Status = "success"
	}

	// Calculate throughput: resources per second
	if m.Duration > 0 {
		m.ThroughputPerSec = float64(resourceCount) / m.Duration.Seconds()
	}
}

// GenerateSyncSummary creates a summary of the sync operation
func GenerateSyncSummary(
	context string,
	startTime time.Time,
	concurrencyLevel int,
	metrics []GVKFetchMetrics,
) SyncSummary {
	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	summary := SyncSummary{
		Context:          context,
		StartTime:        startTime,
		EndTime:          endTime,
		TotalDuration:    totalDuration,
		ConcurrencyLevel: concurrencyLevel,
		TotalGVKs:        len(metrics),
		GVKMetrics:       metrics,
		FailedDetails:    make(map[string]string),
	}

	var totalThroughput float64
	var estimatedSequential time.Duration

	for _, m := range metrics {
		summary.TotalResources += m.ResourceCount

		if m.Status == "success" {
			summary.SuccessfulGVKs++
		} else {
			summary.FailedGVKs++
			summary.FailedDetails[m.GVK] = m.ErrorMessage
		}

		totalThroughput += m.ThroughputPerSec
		estimatedSequential += m.Duration
	}

	if summary.TotalGVKs > 0 {
		summary.AverageThroughput = totalThroughput / float64(summary.TotalGVKs)
	}

	summary.EstimatedSequential = estimatedSequential

	return summary
}

// SpeedupFactor calculates the speedup ratio (estimated sequential / actual concurrent)
func (s *SyncSummary) SpeedupFactor() float64 {
	if s.TotalDuration > 0 {
		return s.EstimatedSequential.Seconds() / s.TotalDuration.Seconds()
	}
	return 0
}

// SortMetricsByDuration sorts metrics by duration (slowest first)
func (s *SyncSummary) SortMetricsByDuration() {
	sort.Slice(s.GVKMetrics, func(i, j int) bool {
		return s.GVKMetrics[i].Duration > s.GVKMetrics[j].Duration
	})
}

// SortMetricsByResourceCount sorts metrics by resource count (highest first)
func (s *SyncSummary) SortMetricsByResourceCount() {
	sort.Slice(s.GVKMetrics, func(i, j int) bool {
		return s.GVKMetrics[i].ResourceCount > s.GVKMetrics[j].ResourceCount
	})
}

// String returns a human-readable summary string
func (s *SyncSummary) String() string {
	output := fmt.Sprintf(`
=== Custom Resources Sync Summary ===
Context:               %s
Total Duration:        %v
Estimated Sequential:  %v
Speedup Factor:        %.2fx
Concurrency Level:     %d

GVK Statistics:
  Total GVKs:          %d
  Successful:          %d
  Failed:              %d

Resource Statistics:
  Total Resources:     %d
  Average Throughput:  %.2f resources/sec

`, s.Context, s.TotalDuration, s.EstimatedSequential, s.SpeedupFactor(),
		s.ConcurrencyLevel, s.TotalGVKs, s.SuccessfulGVKs, s.FailedGVKs,
		s.TotalResources, s.AverageThroughput)

	// Sort by duration and show slowest GVKs
	s.SortMetricsByDuration()
	if len(s.GVKMetrics) > 0 {
		output += "\nSlowest GVKs:\n"
		limit := 5
		if len(s.GVKMetrics) < limit {
			limit = len(s.GVKMetrics)
		}
		for i := 0; i < limit; i++ {
			m := s.GVKMetrics[i]
			status := "✓"
			if m.Status != "success" {
				status = "✗"
			}
			output += fmt.Sprintf("  [%s] %s: %v (%d resources, %.2f/sec)\n",
				status, m.GVK, m.Duration, m.ResourceCount, m.ThroughputPerSec)
		}
	}

	// Show failed GVKs
	if len(s.FailedDetails) > 0 {
		output += "\nFailed GVKs:\n"
		for gvk, errMsg := range s.FailedDetails {
			output += fmt.Sprintf("  ✗ %s: %s\n", gvk, errMsg)
		}
	}

	output += "\n"
	return output
}
