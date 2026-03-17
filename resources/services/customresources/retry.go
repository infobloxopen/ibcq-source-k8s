package customresources

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// IsTransientError checks if an error is likely transient and should be retried.
// Transient errors include timeouts, connection issues, and API rate limits.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check for context deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Context cancelled is not transient - stop retrying
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Check for Kubernetes API status errors
	// Use type assertion to check for APIStatus
	if statusErr, ok := err.(apierrors.APIStatus); ok {
		code := statusErr.Status().Code

		// Transient HTTP status codes that should be retried
		switch code {
		case 429: // Rate limit / Too Many Requests
			return true
		case 500: // Internal Server Error
			return true
		case 502: // Bad Gateway
			return true
		case 503: // Service Unavailable
			return true
		case 504: // Gateway Timeout
			return true
		default:
			return false
		}
	}

	// Check for temporary network errors
	type temporary interface {
		Temporary() bool
	}
	if te, ok := err.(temporary); ok {
		return te.Temporary()
	}

	return false
}

// RetryError represents an error from exhausted retry attempts
type RetryError struct {
	LastErr      error
	Attempts     int
	TotalBackoff time.Duration
}

func (e *RetryError) Error() string {
	return fmt.Sprintf("max retries exceeded (%d attempts, %v total backoff): %v",
		e.Attempts, e.TotalBackoff, e.LastErr)
}

func (e *RetryError) Unwrap() error {
	return e.LastErr
}

// WithRetry wraps a function with exponential backoff retry logic.
// It retries on transient errors and immediately returns on permanent errors.
// Backoff starts at config.RetryBackoff and doubles on each retry, capped at 10 seconds.
func WithRetry(ctx context.Context, config ConcurrencyConfig, fn func() error) error {
	var lastErr error
	backoff := config.RetryBackoff
	totalBackoff := time.Duration(0)

	for attempt := 0; attempt < config.RetryAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil // Success
		}

		// Check if error is transient
		if IsTransientError(err) {
			lastErr = err

			// If this is the last attempt, don't wait
			if attempt < config.RetryAttempts-1 {
				// Wait with context cancellation support
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}

				totalBackoff += backoff

				// Exponential backoff: double the backoff time, cap at 10 seconds
				backoff *= 2
				if backoff > 10*time.Second {
					backoff = 10 * time.Second
				}
			}
		} else {
			// Non-transient error - return immediately without retrying
			return err
		}
	}

	// All retries exhausted
	return &RetryError{
		LastErr:      lastErr,
		Attempts:     config.RetryAttempts,
		TotalBackoff: totalBackoff,
	}
}

// RetryWithMetrics is like WithRetry but also records attempt count and backoff in metrics
func RetryWithMetrics(ctx context.Context, config ConcurrencyConfig, fn func() error, metric *GVKFetchMetrics) error {
	var lastErr error
	backoff := config.RetryBackoff
	totalBackoff := time.Duration(0)
	attempts := 0

	for attempt := 0; attempt < config.RetryAttempts; attempt++ {
		attempts++
		err := fn()
		if err == nil {
			metric.RetryCount = attempt // Record how many retries were needed
			return nil
		}

		// Check if error is transient
		if IsTransientError(err) {
			lastErr = err

			// If this is the last attempt, don't wait
			if attempt < config.RetryAttempts-1 {
				// Wait with context cancellation support
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}

				totalBackoff += backoff

				// Exponential backoff: double the backoff time, cap at 10 seconds
				backoff *= 2
				if backoff > 10*time.Second {
					backoff = 10 * time.Second
				}
			}
		} else {
			// Non-transient error - return immediately without retrying
			metric.RetryCount = attempt
			return err
		}
	}

	// All retries exhausted
	metric.RetryCount = attempts - 1
	return &RetryError{
		LastErr:      lastErr,
		Attempts:     config.RetryAttempts,
		TotalBackoff: totalBackoff,
	}
}
