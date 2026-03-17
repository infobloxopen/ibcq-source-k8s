package customresources

import (
	"time"
)

// ConcurrencyConfig holds concurrency limits and timeout settings for GVK processing
type ConcurrencyConfig struct {
	// MaxConcurrentGVKs is the maximum number of GVK fetches to run in parallel.
	// Default: 10. Set to 0 for unlimited (not recommended).
	MaxConcurrentGVKs int

	// FetchTimeout is the maximum time to wait for a single GVK fetch operation.
	// This includes all retries. Default: 5 minutes.
	FetchTimeout time.Duration

	// RetryAttempts is the maximum number of retry attempts for transient errors.
	// Default: 3. Set to 0 to disable retries.
	RetryAttempts int

	// RetryBackoff is the initial backoff duration for retries, which increases exponentially.
	// Default: 100ms. Example: 100ms, 200ms, 400ms, 800ms, etc.
	RetryBackoff time.Duration

	// MaxRetryBackoff is the maximum backoff duration. Backoff will not exceed this.
	// Default: 10 seconds.
	MaxRetryBackoff time.Duration
}

// DefaultConcurrencyConfig returns safe default values for concurrency configuration.
// These defaults are suitable for most clusters.
func DefaultConcurrencyConfig() ConcurrencyConfig {
	return ConcurrencyConfig{
		MaxConcurrentGVKs: 10,
		FetchTimeout:      5 * time.Minute,
		RetryAttempts:     3,
		RetryBackoff:      100 * time.Millisecond,
		MaxRetryBackoff:   10 * time.Second,
	}
}

// AdjustConcurrencyForClusterSize scales concurrency based on cluster size.
// This is a best-effort adjustment to avoid overwhelming small clusters
// and maximize throughput on large clusters.
//
// Scaling logic:
//   - Min: 5 workers (even small clusters get some parallelism)
//   - Max: 50 workers (prevent runaway concurrency on huge clusters)
//   - Ratio: 1 worker per 50 nodes
//
// Example:
//   - 100 nodes -> 2 workers (clamped to min 5) -> 5 workers
//   - 500 nodes -> 10 workers
//   - 2500 nodes -> 50 workers (clamped to max 50) -> 50 workers
func AdjustConcurrencyForClusterSize(nodeCount int) int {
	workers := nodeCount / 50

	if workers < 5 {
		workers = 5
	}
	if workers > 50 {
		workers = 50
	}

	return workers
}

// Validate checks if the concurrency config has valid values and logs warnings if not.
func (cc *ConcurrencyConfig) Validate() error {
	if cc.MaxConcurrentGVKs < 0 {
		cc.MaxConcurrentGVKs = 0 // Unlimited
	}

	if cc.FetchTimeout <= 0 {
		cc.FetchTimeout = 5 * time.Minute
	}

	if cc.RetryAttempts < 0 {
		cc.RetryAttempts = 0
	}

	if cc.RetryBackoff <= 0 {
		cc.RetryBackoff = 100 * time.Millisecond
	}

	if cc.MaxRetryBackoff <= 0 {
		cc.MaxRetryBackoff = 10 * time.Second
	}

	if cc.MaxRetryBackoff < cc.RetryBackoff {
		cc.MaxRetryBackoff = cc.RetryBackoff
	}

	return nil
}
