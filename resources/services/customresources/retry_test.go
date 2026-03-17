package customresources

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestIsTransientError_Timeout verifies timeout is treated as transient
func TestIsTransientError_Timeout(t *testing.T) {
	err := context.DeadlineExceeded
	assert.True(t, IsTransientError(err))
}

// TestIsTransientError_Cancelled verifies context cancelled is NOT transient
func TestIsTransientError_Cancelled(t *testing.T) {
	err := context.Canceled
	assert.False(t, IsTransientError(err))
}

// TestIsTransientError_RateLimit verifies 429 is transient
func TestIsTransientError_RateLimit(t *testing.T) {
	err := apierrors.NewTooManyRequests("rate limited", 0)
	assert.True(t, IsTransientError(err))
}

// TestIsTransientError_ServiceUnavailable verifies 503 is transient
func TestIsTransientError_ServiceUnavailable(t *testing.T) {
	err := apierrors.NewServiceUnavailable("service down")
	assert.True(t, IsTransientError(err))
}

// TestIsTransientError_ServerError verifies 500 is transient
func TestIsTransientError_ServerError(t *testing.T) {
	err := apierrors.NewInternalError(fmt.Errorf("server error"))
	assert.True(t, IsTransientError(err))
}

// TestIsTransientError_GatewayTimeout verifies 504 is transient
func TestIsTransientError_GatewayTimeout(t *testing.T) {
	statusErr := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Code: 504,
		},
	}
	assert.True(t, IsTransientError(statusErr))
}

// TestIsTransientError_Forbidden verifies 403 is NOT transient (RBAC)
func TestIsTransientError_Forbidden(t *testing.T) {
	gr := schema.GroupResource{Group: "api.example.com", Resource: "certificates"}
	err := apierrors.NewForbidden(gr, "certificate", fmt.Errorf("RBAC denied"))
	assert.False(t, IsTransientError(err))
}

// TestIsTransientError_NotFound verifies 404 is NOT transient
func TestIsTransientError_NotFound(t *testing.T) {
	gr := schema.GroupResource{Group: "api.example.com", Resource: "certificates"}
	err := apierrors.NewNotFound(gr, "resource")
	assert.False(t, IsTransientError(err))
}

// TestIsTransientError_Nil verifies nil error is not transient
func TestIsTransientError_Nil(t *testing.T) {
	assert.False(t, IsTransientError(nil))
}

// TestWithRetry_Success verifies successful operation returns immediately
func TestWithRetry_Success(t *testing.T) {
	config := DefaultConcurrencyConfig()
	ctx := context.Background()

	callCount := 0
	err := WithRetry(ctx, config, func() error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount) // Should only call once
}

// TestWithRetry_TransientError verifies transient errors are retried
func TestWithRetry_TransientError(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 3
	config.RetryBackoff = 10 * time.Millisecond // Short for test
	ctx := context.Background()

	callCount := 0
	err := WithRetry(ctx, config, func() error {
		callCount++
		if callCount < 3 {
			return context.DeadlineExceeded // Transient
		}
		return nil // Success on 3rd attempt
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount) // Should retry twice then succeed
}

// TestWithRetry_PermanentError verifies permanent errors are not retried
func TestWithRetry_PermanentError(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 3
	ctx := context.Background()

	callCount := 0
	gr := schema.GroupResource{Group: "api.example.com", Resource: "certificates"}
	permanentErr := apierrors.NewForbidden(gr, "certificate", fmt.Errorf("RBAC denied"))
	err := WithRetry(ctx, config, func() error {
		callCount++
		return permanentErr
	})

	assert.Equal(t, permanentErr, err)
	assert.Equal(t, 1, callCount) // Should not retry
}

// TestWithRetry_ExhaustedRetries verifies max retries error is returned
func TestWithRetry_ExhaustedRetries(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 3
	config.RetryBackoff = 5 * time.Millisecond
	ctx := context.Background()

	callCount := 0
	err := WithRetry(ctx, config, func() error {
		callCount++
		return context.DeadlineExceeded // Always transient
	})

	assert.Error(t, err)
	assert.Equal(t, 3, callCount) // Should attempt 3 times
	assert.Contains(t, err.Error(), "max retries exceeded")

	// Verify it's a RetryError
	var retryErr *RetryError
	assert.ErrorAs(t, err, &retryErr)
	assert.Equal(t, 3, retryErr.Attempts)
}

// TestWithRetry_ContextCancelled verifies context cancellation stops retries
func TestWithRetry_ContextCancelled(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 3
	config.RetryBackoff = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	err := WithRetry(ctx, config, func() error {
		callCount++
		if callCount == 2 {
			cancel() // Cancel after 1st failure
		}
		return context.DeadlineExceeded
	})

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 2, callCount) // Should stop after context cancelled
}

// TestWithRetry_ExponentialBackoff verifies backoff timing
func TestWithRetry_ExponentialBackoff(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 4
	config.RetryBackoff = 10 * time.Millisecond
	ctx := context.Background()

	times := []time.Time{}
	err := WithRetry(ctx, config, func() error {
		times = append(times, time.Now())
		if len(times) < 4 {
			return context.DeadlineExceeded // Retry
		}
		return nil
	})

	assert.NoError(t, err)
	require.Len(t, times, 4)

	// Verify backoff increases exponentially
	// First wait: ~10ms, Second wait: ~20ms, Third wait: ~40ms
	delay1 := times[1].Sub(times[0])
	delay2 := times[2].Sub(times[1])
	delay3 := times[3].Sub(times[2])

	// Each delay should roughly be 2x the previous (with some tolerance for timing)
	assert.Greater(t, delay2.Milliseconds(), delay1.Milliseconds())
	assert.Greater(t, delay3.Milliseconds(), delay2.Milliseconds())
}

// TestWithRetry_BackoffCap verifies backoff is capped at 10 seconds
func TestWithRetry_BackoffCap(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 6
	config.RetryBackoff = 1 * time.Second
	ctx := context.Background()

	times := []time.Time{}
	err := WithRetry(ctx, config, func() error {
		times = append(times, time.Now())
		if len(times) < 6 {
			return context.DeadlineExceeded
		}
		return nil
	})

	assert.NoError(t, err)
	require.Len(t, times, 6)

	// After several retries, backoff should be capped at 10 seconds
	// Last delay should be around 10 seconds (not 32 seconds which would be 2^5)
	delay5 := times[5].Sub(times[4])
	assert.Less(t, delay5.Milliseconds(), int64(11000)) // Should be ~10s, not 32s
}

// TestRetryWithMetrics_Success verifies metrics are updated on success
func TestRetryWithMetrics_Success(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 3
	config.RetryBackoff = 5 * time.Millisecond
	ctx := context.Background()

	metric := NewGVKFetchMetrics("test.io/v1/Resource")
	callCount := 0

	err := RetryWithMetrics(ctx, config, func() error {
		callCount++
		if callCount < 3 {
			return context.DeadlineExceeded
		}
		return nil
	}, metric)

	assert.NoError(t, err)
	assert.Equal(t, 2, metric.RetryCount) // 2 retries before success
}

// TestRetryWithMetrics_PermanentError verifies metrics on permanent error
func TestRetryWithMetrics_PermanentError(t *testing.T) {
	config := DefaultConcurrencyConfig()
	ctx := context.Background()

	metric := NewGVKFetchMetrics("test.io/v1/Resource")
	gr := schema.GroupResource{Group: "api.example.com", Resource: "resources"}
	rbacErr := apierrors.NewForbidden(gr, "res", fmt.Errorf("no permission"))

	err := RetryWithMetrics(ctx, config, func() error {
		return rbacErr
	}, metric)

	assert.Equal(t, rbacErr, err)
	assert.Equal(t, 0, metric.RetryCount) // No retries on permanent error
}

// TestRetryWithMetrics_ExhaustedRetries verifies metrics on exhausted retries
func TestRetryWithMetrics_ExhaustedRetries(t *testing.T) {
	config := DefaultConcurrencyConfig()
	config.RetryAttempts = 3
	config.RetryBackoff = 2 * time.Millisecond
	ctx := context.Background()

	metric := NewGVKFetchMetrics("test.io/v1/Resource")

	err := RetryWithMetrics(ctx, config, func() error {
		return context.DeadlineExceeded
	}, metric)

	assert.Error(t, err)
	assert.Equal(t, 2, metric.RetryCount) // 2 retries before giving up
}

// TestRetryError_Unwrap verifies RetryError implements error interface
func TestRetryError_Unwrap(t *testing.T) {
	underlyingErr := fmt.Errorf("underlying timeout")
	retryErr := &RetryError{
		LastErr:      underlyingErr,
		Attempts:     3,
		TotalBackoff: 35 * time.Millisecond,
	}

	// Verify error message
	assert.Contains(t, retryErr.Error(), "max retries exceeded")
	assert.Contains(t, retryErr.Error(), "3 attempts")

	// Verify Unwrap
	assert.Equal(t, underlyingErr, retryErr.Unwrap())

	// Verify it's an error
	var _ error = retryErr
}
