package customresources

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FetchResult holds the outcome of fetching resources for a single GVK.
// It captures both success and failure information, allowing partial failures
// to not block other GVKs from being processed.
type FetchResult struct {
	// GVK is the Group/Version/Kind identifier (e.g., "cert-manager.io/v1/Certificate")
	GVK string

	// Resources contains the fetched unstructured resources
	Resources []unstructured.Unstructured

	// Error is set if the fetch failed. May be non-nil even if Resources is non-empty (partial success).
	Error error

	// Duration is the total time taken for this GVK fetch, including retries.
	Duration time.Duration

	// Attempts is the number of fetch attempts made (1 = no retries, >1 = retried).
	Attempts int

	// Status is a human-readable status: "success", "failed", "partial"
	Status string
}

// IsSuccess returns true if the fetch completed successfully (no error).
func (fr *FetchResult) IsSuccess() bool {
	return fr.Error == nil
}

// IsFailed returns true if the fetch failed (has error).
func (fr *FetchResult) IsFailed() bool {
	return fr.Error != nil
}

// ResourceCount returns the number of resources fetched.
func (fr *FetchResult) ResourceCount() int {
	return len(fr.Resources)
}

// Throughput returns the resources fetched per second.
// Returns 0 if Duration is 0 to avoid division by zero.
func (fr *FetchResult) Throughput() float64 {
	if fr.Duration == 0 {
		return 0
	}
	return float64(fr.ResourceCount()) / fr.Duration.Seconds()
}

// GVKError represents an error from a single GVK fetch that didn't block other GVKs.
type GVKError struct {
	// GVK is the Group/Version/Kind that failed
	GVK string

	// Err is the underlying error
	Err error

	// Attempts is the number of retry attempts made
	Attempts int

	// ErrorType classifies the error: "rbac", "timeout", "api_error", "validation", "unknown"
	ErrorType string

	// RetryableError indicates if the error could be retried
	RetryableError bool
}

// Error implements the error interface
func (ge *GVKError) Error() string {
	if ge.Err == nil {
		return "unknown error for GVK " + ge.GVK
	}
	return ge.Err.Error()
}

// Unwrap returns the underlying error
func (ge *GVKError) Unwrap() error {
	return ge.Err
}
