package customresources

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudquery/cloudquery/plugins/source/k8s/client"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1schema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CustomResources returns the table definition for k8s_custom_resources
func CustomResources() *schema.Table {
	return &schema.Table{
		Name:      "k8s_custom_resources",
		Resolver:  fetchCustomResources,
		Multiplex: client.ContextMultiplex,
		Transform: transformers.TransformWithStruct(&CustomResourceRow{}, transformers.WithPrimaryKeys("UID")),
		Columns:   schema.ColumnList{client.ContextColumn},
	}
}

// CustomResourceRow represents a single custom resource in the table
type CustomResourceRow struct {
	Context         string            `json:"context"`
	APIVersion      string            `json:"api_version"`
	Kind            string            `json:"kind"`
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	UID             string            `json:"uid"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
	OwnerReferences []any             `json:"owner_references"`
	Finalizers      []string          `json:"finalizers"`
	Spec            map[string]any    `json:"spec"`
	Status          map[string]any    `json:"status,omitempty"`
}

// fetchCustomResources retrieves custom resources based on plugin configuration using concurrent processing.
// Multiple GVKs are fetched in parallel with bounded concurrency to improve performance while
// controlling resource usage. If one GVK fails, others continue processing (partial success).
// Includes per-GVK timing logs and sync summary report for observability.
func fetchCustomResources(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource, res chan<- any) error {
	cl := meta.(*client.Client)

	// Get custom resources from configuration
	spec := cl.Spec()
	if spec == nil || len(spec.CustomResources) == 0 {
		// No custom resources configured, skip
		return nil
	}

	// Create concurrency config with sensible defaults
	config := DefaultConcurrencyConfig()

	// Start sync timer for metrics
	syncStartTime := time.Now()

	// Create errgroup with bounded concurrency for parallel GVK processing
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(config.MaxConcurrentGVKs)

	// Use a mutex to safely send results to the channel
	// This prevents concurrent writes to the res channel which could cause panics
	var mu sync.Mutex

	// Track results for each GVK to handle partial failures gracefully
	results := make([]FetchResult, len(spec.CustomResources))
	var gvkErrors []GVKError

	// Collect metrics for each GVK
	metrics := make([]GVKFetchMetrics, len(spec.CustomResources))

	// Process each configured custom resource concurrently
	for i, crSpec := range spec.CustomResources {
		i, crSpec := i, crSpec // Capture loop variables for goroutine closure

		g.Go(func() error {
			// Create a context with timeout for this GVK fetch
			fetchCtx, cancel := context.WithTimeout(ctx, config.FetchTimeout)
			defer cancel()

			// Start timing for this GVK
			startTime := time.Now()
			gvkMetric := NewGVKFetchMetrics(crSpec.GVK)
			gvkMetric.StartTime = startTime

			// Parse GVK to GVR
			gvr, err := parseGVKToGVR(crSpec.GVK)
			if err != nil {
				results[i] = FetchResult{
					GVK:      crSpec.GVK,
					Error:    fmt.Errorf("invalid GVK format: %w", err),
					Duration: time.Since(startTime),
					Attempts: 1,
					Status:   "failed",
				}
				gvkMetric.Complete(0, err)
				metrics[i] = *gvkMetric

				gvkErrors = append(gvkErrors, GVKError{
					GVK:       crSpec.GVK,
					Err:       err,
					Attempts:  1,
					ErrorType: "validation",
				})

				// Log failed GVK fetch
				cl.Logger().Debug().Err(err).Str("gvk", crSpec.GVK).Msg("Failed to parse GVK")
				return nil // Don't fail entire fetch
			}

			// Fetch resources for this GVK with retry for transient errors
			var resources []unstructured.Unstructured
			err = RetryWithMetrics(fetchCtx, config, func() error {
				var fetchErr error
				resources, fetchErr = fetchResourcesByGVRConcurrent(fetchCtx, cl, crSpec.GVK, gvr, crSpec)
				return fetchErr
			}, gvkMetric)

			duration := time.Since(startTime)

			if err != nil {
				results[i] = FetchResult{
					GVK:       crSpec.GVK,
					Resources: resources,
					Error:     err,
					Duration:  duration,
					Attempts:  gvkMetric.RetryCount + 1,
					Status:    "failed",
				}
				gvkMetric.Complete(len(resources), err)
				metrics[i] = *gvkMetric

				gvkErrors = append(gvkErrors, GVKError{
					GVK:      crSpec.GVK,
					Err:      err,
					Attempts: gvkMetric.RetryCount + 1,
				})

				// Log failed GVK fetch with timing and retry info
				cl.Logger().Warn().
					Err(err).
					Str("gvk", crSpec.GVK).
					Dur("duration_ms", duration).
					Int("retry_attempts", gvkMetric.RetryCount).
					Msg("GVK fetch failed after retries")
				return nil // Don't fail entire fetch
			}

			results[i] = FetchResult{
				GVK:       crSpec.GVK,
				Resources: resources,
				Error:     nil,
				Duration:  duration,
				Attempts:  gvkMetric.RetryCount + 1,
				Status:    "success",
			}
			gvkMetric.Complete(len(resources), nil)
			metrics[i] = *gvkMetric

			// Log successful GVK fetch with metrics and retry info
			cl.Logger().Info().
				Str("gvk", crSpec.GVK).
				Str("status", "success").
				Dur("duration_ms", duration).
				Int("resource_count", len(resources)).
				Int("retry_attempts", gvkMetric.RetryCount).
				Float64("throughput_per_sec", gvkMetric.ThroughputPerSec).
				Msg("GVK fetch completed")

			return nil
		})
	}

	// Wait for all GVK fetches to complete
	if err := g.Wait(); err != nil {
		// errgroup only returns error if context was cancelled
		return fmt.Errorf("concurrent fetch error: %w", err)
	}

	// Send fetched resources to the output channel (sequentially to maintain order)
	successCount := 0
	totalResources := 0
	for _, result := range results {
		if result.Error == nil {
			successCount++
			totalResources += len(result.Resources)
			for _, resource := range result.Resources {
				row, err := convertToCustomResourceRow(ctx, cl, result.GVK, &resource)
				if err != nil {
					// Log and continue on conversion error
					continue
				}

				mu.Lock()
				res <- row
				mu.Unlock()
			}
		}
	}

	// Generate and log sync summary with metrics
	// Use cl.Context to get the actual Kubernetes context name
	contextName := cl.Context
	if contextName == "" {
		contextName = "unknown"
	}

	summary := GenerateSyncSummary(contextName, syncStartTime, config.MaxConcurrentGVKs, metrics)

	// Log summary with key metrics
	cl.Logger().Info().
		Str("context", summary.Context).
		Int("total_gvks", summary.TotalGVKs).
		Int("successful_gvks", summary.SuccessfulGVKs).
		Int("failed_gvks", summary.FailedGVKs).
		Int("total_resources", summary.TotalResources).
		Dur("total_duration", summary.TotalDuration).
		Dur("estimated_sequential", summary.EstimatedSequential).
		Float64("speedup_factor", summary.SpeedupFactor()).
		Float64("avg_throughput", summary.AverageThroughput).
		Msg("Custom resources sync completed")

	// Print human-readable summary
	cl.Logger().Info().Msg(summary.String())

	// If all GVKs failed, return an error
	if successCount == 0 && len(gvkErrors) > 0 {
		return fmt.Errorf("all custom resources fetch failed: %d errors encountered", len(gvkErrors))
	}

	return nil
}

// parseGVKToGVR converts "group/version/kind" to GroupVersionResource
// This is a helper function that will be used in Phase 4
func parseGVKToGVR(gvk string) (metav1schema.GroupVersionResource, error) {
	parts := strings.Split(gvk, "/")
	if len(parts) != 3 {
		return metav1schema.GroupVersionResource{}, fmt.Errorf("invalid GVK format: %s", gvk)
	}

	group, version, kind := parts[0], parts[1], parts[2]

	// Convert Kind to resource name (e.g., Certificate -> certificates)
	// This is a simple pluralization - may need enhancement for irregular plurals
	resource := strings.ToLower(kind) + "s"

	return metav1schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}

// fetchResourcesByGVRConcurrent fetches all resources for a given GVR and returns them as a slice.
// This is used internally by concurrent processing and doesn't send directly to the output channel.
func fetchResourcesByGVRConcurrent(ctx context.Context, cl *client.Client, gvk string, gvr metav1schema.GroupVersionResource, crSpec spec.CustomResourceSpec) ([]unstructured.Unstructured, error) {
	dynClient := cl.DynamicClient()
	if dynClient == nil {
		return nil, fmt.Errorf("dynamic client not initialized for context %q", cl.Context)
	}

	var allResources []unstructured.Unstructured

	// Determine if we need to filter by namespaces
	if len(crSpec.Namespaces) > 0 {
		// Fetch from specific namespaces
		for _, ns := range crSpec.Namespaces {
			resources, err := fetchFromNamespaceConcurrent(ctx, cl, dynClient.Resource(gvr).Namespace(ns), gvk)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch from namespace %q: %w", ns, err)
			}
			allResources = append(allResources, resources...)
		}
	} else {
		// Fetch from all namespaces (or cluster-scoped if applicable)
		resources, err := fetchFromNamespaceConcurrent(ctx, cl, dynClient.Resource(gvr).Namespace(""), gvk)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch resources: %w", err)
		}
		allResources = append(allResources, resources...)
	}

	return allResources, nil
}

// fetchFromNamespaceConcurrent handles pagination and fetching from a specific namespace (or all if empty).
// Returns a slice of resources instead of sending to a channel to support concurrent processing.
func fetchFromNamespaceConcurrent(ctx context.Context, cl *client.Client, resourceClient dynamic.ResourceInterface, gvk string) ([]unstructured.Unstructured, error) {
	var allResources []unstructured.Unstructured
	opts := metav1.ListOptions{}

	for {
		list, err := resourceClient.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list resources: %w", err)
		}

		allResources = append(allResources, list.Items...)

		// Check for pagination
		if list.GetContinue() == "" {
			break
		}
		opts.Continue = list.GetContinue()
	}

	return allResources, nil
}

// fetchResourcesByGVR fetches all resources for a given GVR (legacy function, kept for compatibility)
func fetchResourcesByGVR(ctx context.Context, cl *client.Client, gvk string, gvr metav1schema.GroupVersionResource, crSpec spec.CustomResourceSpec, res chan<- any) error {
	dynClient := cl.DynamicClient()
	if dynClient == nil {
		return fmt.Errorf("dynamic client not initialized for context %q", cl.Context)
	}

	// Determine if we need to filter by namespaces
	if len(crSpec.Namespaces) > 0 {
		// Fetch from specific namespaces
		for _, ns := range crSpec.Namespaces {
			if err := fetchFromNamespace(ctx, cl, dynClient.Resource(gvr).Namespace(ns), gvk, res); err != nil {
				return fmt.Errorf("failed to fetch from namespace %q: %w", ns, err)
			}
		}
	} else {
		// Fetch from all namespaces (or cluster-scoped if applicable)
		if err := fetchFromNamespace(ctx, cl, dynClient.Resource(gvr).Namespace(""), gvk, res); err != nil {
			return fmt.Errorf("failed to fetch resources: %w", err)
		}
	}

	return nil
}

// fetchFromNamespace handles pagination and fetching from a specific namespace (or all if empty)
func fetchFromNamespace(ctx context.Context, cl *client.Client, resourceClient dynamic.ResourceInterface, gvk string, res chan<- any) error {
	opts := metav1.ListOptions{}

	for {
		list, err := resourceClient.List(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to list resources: %w", err)
		}

		// Convert each item to CustomResourceRow
		for i := range list.Items {
			row, err := convertToCustomResourceRow(ctx, cl, gvk, &list.Items[i])
			if err != nil {
				return fmt.Errorf("failed to convert resource %q: %w", list.Items[i].GetName(), err)
			}
			res <- row
		}

		// Check for pagination
		if list.GetContinue() == "" {
			break
		}
		opts.Continue = list.GetContinue()
	}

	return nil
}

// convertToCustomResourceRow converts an unstructured object to CustomResourceRow
func convertToCustomResourceRow(ctx context.Context, cl *client.Client, gvk string, obj *unstructured.Unstructured) (*CustomResourceRow, error) {
	if obj == nil {
		return nil, fmt.Errorf("unstructured object is nil")
	}

	row := &CustomResourceRow{
		Context:    cl.Context,
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
		UID:        string(obj.GetUID()),
	}

	// Set labels (will be stored as jsonb in postgres)
	if labels := obj.GetLabels(); labels != nil {
		row.Labels = labels
	}

	// Set annotations (will be stored as jsonb in postgres)
	if annotations := obj.GetAnnotations(); annotations != nil {
		row.Annotations = annotations
	}

	// Set owner references
	if ownerRefs := obj.GetOwnerReferences(); ownerRefs != nil {
		refs := make([]any, len(ownerRefs))
		for i, ref := range ownerRefs {
			refs[i] = ref
		}
		row.OwnerReferences = refs
	}

	// Set finalizers
	row.Finalizers = obj.GetFinalizers()

	// Extract spec (will be stored as jsonb in postgres)
	if spec, found, err := unstructured.NestedMap(obj.Object, "spec"); err != nil {
		return nil, fmt.Errorf("failed to extract spec: %w", err)
	} else if found && spec != nil {
		row.Spec = spec
	}

	// Extract status (will be stored as jsonb in postgres)
	if status, found, err := unstructured.NestedMap(obj.Object, "status"); err != nil {
		return nil, fmt.Errorf("failed to extract status: %w", err)
	} else if found && status != nil {
		row.Status = status
	}

	return row, nil
}
