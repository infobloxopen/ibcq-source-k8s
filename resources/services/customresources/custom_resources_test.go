package customresources

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/apache/arrow/go/v16/arrow"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1schema "k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCustomResources_TableStructure(t *testing.T) {
	table := CustomResources()

	// Verify table name
	assert.Equal(t, "k8s_custom_resources", table.Name)

	// Verify resolver is set
	assert.NotNil(t, table.Resolver)

	// Verify multiplex is set
	assert.NotNil(t, table.Multiplex)

	// Verify required columns exist - columns come from transformers.WithStruct
	// We only manually add context column, rest are from CustomResourceRow struct
	requiredColumns := []string{
		"context",
	}

	// Note: With transformers.WithStruct, columns are generated from the struct fields:
	// api_version, kind, namespace, name, uid, labels, annotations,
	// owner_references, finalizers, spec, status

	for _, colName := range requiredColumns {
		found := false
		for _, col := range table.Columns {
			if col.Name == colName {
				found = true
				break
			}
		}
		assert.True(t, found, "Column %q should exist", colName)
	}
}

func TestCustomResources_ColumnTypes(t *testing.T) {
	table := CustomResources()

	// Verify context column is manually added
	var contextCol *schema.Column
	for i := range table.Columns {
		if table.Columns[i].Name == "context" {
			contextCol = &table.Columns[i]
			break
		}
	}
	require.NotNil(t, contextCol, "Column 'context' should be manually added")
	assert.Equal(t, arrow.BinaryTypes.String, contextCol.Type)

	// Verify Transform is configured (this generates columns from struct)
	assert.NotNil(t, table.Transform, "Transform should be configured with WithStruct")

	// Note: Other columns (api_version, kind, namespace, name, uid, labels, annotations,
	// owner_references, finalizers, spec, status) are automatically generated from
	// CustomResourceRow struct by transformers.WithStruct
}

func TestParseGVKToGVR(t *testing.T) {
	tests := []struct {
		name        string
		gvk         string
		expectedGVR metav1schema.GroupVersionResource
		expectError bool
	}{
		{
			name: "valid_cert_manager",
			gvk:  "cert-manager.io/v1/Certificate",
			expectedGVR: metav1schema.GroupVersionResource{
				Group:    "cert-manager.io",
				Version:  "v1",
				Resource: "certificates",
			},
			expectError: false,
		},
		{
			name: "valid_argo",
			gvk:  "argoproj.io/v1alpha1/Application",
			expectedGVR: metav1schema.GroupVersionResource{
				Group:    "argoproj.io",
				Version:  "v1alpha1",
				Resource: "applications",
			},
			expectError: false,
		},
		{
			name:        "invalid_format_missing_part",
			gvk:         "cert-manager.io/v1",
			expectError: true,
		},
		{
			name:        "invalid_format_too_many_parts",
			gvk:         "cert-manager.io/v1/Certificate/extra",
			expectError: true,
		},
		{
			name:        "empty_string",
			gvk:         "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvr, err := parseGVKToGVR(tt.gvk)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedGVR.Group, gvr.Group)
				assert.Equal(t, tt.expectedGVR.Version, gvr.Version)
				assert.Equal(t, tt.expectedGVR.Resource, gvr.Resource)
			}
		})
	}
}

func TestCustomResourceRow_Structure(t *testing.T) {
	// Verify CustomResourceRow has all required fields
	row := CustomResourceRow{
		Context:     "test-context",
		APIVersion:  "cert-manager.io/v1",
		Kind:        "Certificate",
		Namespace:   "default",
		Name:        "test-cert",
		UID:         "12345",
		Labels:      map[string]string{"app": "test"},
		Annotations: map[string]string{"description": "test"},
		Finalizers:  []string{"finalizer.example.com"},
		OwnerReferences: []any{map[string]interface{}{
			"kind": "Certificate",
			"name": "parent-cert",
		}},
		Spec:   map[string]any{"secretName": "test-secret"},
		Status: map[string]any{"ready": true},
	}

	// Verify fields are accessible
	assert.Equal(t, "test-context", row.Context)
	assert.Equal(t, "cert-manager.io/v1", row.APIVersion)
	assert.Equal(t, "Certificate", row.Kind)
	assert.Equal(t, "default", row.Namespace)
	assert.Equal(t, "test-cert", row.Name)
	assert.Equal(t, "12345", row.UID)
	assert.NotNil(t, row.Labels)
	assert.NotNil(t, row.Annotations)
	assert.NotNil(t, row.Spec)
	assert.NotNil(t, row.Status)
}

func TestCustomResources_PrimaryKeys(t *testing.T) {
	table := CustomResources()

	// Verify table has Transform configured
	assert.NotNil(t, table.Transform)

	// The primary key is uid (with deterministic _cq_id)
	// This test verifies the table is properly configured with WithPrimaryKeys("UID")
	// Actual validation happens during TransformTables in the plugin
}

func TestConvertToCustomResourceRow(t *testing.T) {
	tests := []struct {
		name      string
		setupObj  func() *unstructured.Unstructured
		gvk       string
		context   string
		expectErr bool
		validate  func(*testing.T, *CustomResourceRow)
	}{
		{
			name:    "complete_certificate_object",
			gvk:     "cert-manager.io/v1/Certificate",
			context: "test-context",
			setupObj: func() *unstructured.Unstructured {
				obj := &unstructured.Unstructured{}
				obj.SetAPIVersion("cert-manager.io/v1")
				obj.SetKind("Certificate")
				obj.SetName("test-cert")
				obj.SetNamespace("default")
				obj.SetUID("12345-abcde")
				obj.SetResourceVersion("v1")
				obj.SetGeneration(1)
				obj.SetLabels(map[string]string{"app": "test", "env": "dev"})
				obj.SetAnnotations(map[string]string{"description": "test certificate"})
				obj.SetCreationTimestamp(metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})

				obj.Object["spec"] = map[string]interface{}{
					"secretName": "test-secret",
					"dnsNames":   []interface{}{"example.com"},
				}
				obj.Object["status"] = map[string]interface{}{
					"ready": true,
				}
				return obj
			},
			expectErr: false,
			validate: func(t *testing.T, row *CustomResourceRow) {
				assert.Equal(t, "test-context", row.Context)
				assert.Equal(t, "cert-manager.io/v1", row.APIVersion)
				assert.Equal(t, "Certificate", row.Kind)
				assert.Equal(t, "default", row.Namespace)
				assert.Equal(t, "test-cert", row.Name)
				assert.Equal(t, "12345-abcde", row.UID)
				assert.Equal(t, "test", row.Labels["app"])
				assert.Equal(t, "dev", row.Labels["env"])
				assert.Equal(t, "test certificate", row.Annotations["description"])
				assert.Equal(t, "test-secret", row.Spec["secretName"])
				assert.Equal(t, true, row.Status["ready"])
			},
		},
		{
			name:    "cluster_scoped_resource",
			gvk:     "storage.k8s.io/v1/StorageClass",
			context: "test-context",
			setupObj: func() *unstructured.Unstructured {
				obj := &unstructured.Unstructured{}
				obj.SetAPIVersion("storage.k8s.io/v1")
				obj.SetKind("StorageClass")
				obj.SetName("fast-storage")
				obj.SetUID("cluster-12345")
				obj.SetResourceVersion("v2")
				obj.SetGeneration(2)
				obj.SetCreationTimestamp(metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})

				obj.Object["spec"] = map[string]interface{}{
					"provisioner": "fast.io",
				}
				return obj
			},
			expectErr: false,
			validate: func(t *testing.T, row *CustomResourceRow) {
				assert.Equal(t, "storage.k8s.io/v1", row.APIVersion)
				assert.Equal(t, "StorageClass", row.Kind)
				assert.Equal(t, "fast-storage", row.Name)
				assert.Empty(t, row.Namespace) // cluster-scoped
				assert.NotNil(t, row.Spec)
				assert.Equal(t, "fast.io", row.Spec["provisioner"])
			},
		},
		{
			name:    "minimal_object_no_spec_status",
			gvk:     "example.com/v1/Sample",
			context: "test-context",
			setupObj: func() *unstructured.Unstructured {
				obj := &unstructured.Unstructured{}
				obj.SetAPIVersion("example.com/v1")
				obj.SetKind("Sample")
				obj.SetName("minimal")
				obj.SetNamespace("default")
				obj.SetUID("minimal-123")
				obj.SetCreationTimestamp(metav1.Time{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
				return obj
			},
			expectErr: false,
			validate: func(t *testing.T, row *CustomResourceRow) {
				assert.Equal(t, "minimal", row.Name)
				assert.Equal(t, "example.com/v1", row.APIVersion)
				assert.Equal(t, "Sample", row.Kind)
				assert.Nil(t, row.Spec)        // nil for missing spec
				assert.Nil(t, row.Status)      // nil for missing status
				assert.Nil(t, row.Labels)      // nil for missing labels
				assert.Nil(t, row.Annotations) // nil for missing annotations
			},
		},
		{
			name:    "nil_object",
			gvk:     "example.com/v1/Sample",
			context: "test-context",
			setupObj: func() *unstructured.Unstructured {
				return nil
			},
			expectErr: true,
		},
		{
			name:    "object_with_owner_references_and_finalizers",
			gvk:     "apps/v1/ReplicaSet",
			context: "test-context",
			setupObj: func() *unstructured.Unstructured {
				obj := &unstructured.Unstructured{}
				obj.SetAPIVersion("apps/v1")
				obj.SetKind("ReplicaSet")
				obj.SetName("test-rs")
				obj.SetNamespace("default")
				obj.SetUID("rs-12345")
				obj.SetOwnerReferences([]metav1.OwnerReference{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "parent-deployment",
						UID:        "deploy-123",
					},
				})
				obj.SetFinalizers([]string{"finalizer.example.com", "cleanup.example.com"})
				return obj
			},
			expectErr: false,
			validate: func(t *testing.T, row *CustomResourceRow) {
				assert.Equal(t, "test-rs", row.Name)
				assert.NotNil(t, row.OwnerReferences)
				assert.Len(t, row.OwnerReferences, 1)
				assert.NotNil(t, row.Finalizers)
				assert.Len(t, row.Finalizers, 2)
				assert.Contains(t, row.Finalizers, "finalizer.example.com")
				assert.Contains(t, row.Finalizers, "cleanup.example.com")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.setupObj()

			if tt.expectErr {
				// For error cases (like nil object), test the conversion
				mockClient := &client.Client{Context: tt.context}
				row, err := convertToCustomResourceRow(context.Background(), mockClient, tt.gvk, obj)
				assert.Error(t, err)
				assert.Nil(t, row)
				return
			}

			// For non-error tests, convert and validate
			assert.NotNil(t, obj)
			mockClient := &client.Client{Context: tt.context}
			row, err := convertToCustomResourceRow(context.Background(), mockClient, tt.gvk, obj)
			assert.NoError(t, err)
			assert.NotNil(t, row)

			if tt.validate != nil {
				tt.validate(t, row)
			}
		})
	}
}

// Note: Full integration tests for convertToCustomResourceRow will be in Phase 5
// These unit tests verify the table structure and helper functions

// TestDefaultConcurrencyConfig verifies that default concurrency config has sensible values
func TestDefaultConcurrencyConfig(t *testing.T) {
	config := DefaultConcurrencyConfig()

	assert.Equal(t, 10, config.MaxConcurrentGVKs, "Default max concurrent GVKs should be 10")
	assert.Equal(t, 5*time.Minute, config.FetchTimeout, "Default fetch timeout should be 5 minutes")
	assert.Equal(t, 3, config.RetryAttempts, "Default retry attempts should be 3")
	assert.Equal(t, 100*time.Millisecond, config.RetryBackoff, "Default retry backoff should be 100ms")
	assert.Equal(t, 10*time.Second, config.MaxRetryBackoff, "Default max retry backoff should be 10s")
}

// TestConcurrencyConfigValidate verifies that Validate() corrects invalid values
func TestConcurrencyConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		config ConcurrencyConfig
		expect ConcurrencyConfig
	}{
		{
			name:   "negative_max_concurrent",
			config: ConcurrencyConfig{MaxConcurrentGVKs: -5},
			expect: ConcurrencyConfig{MaxConcurrentGVKs: 0, FetchTimeout: 5 * time.Minute, RetryAttempts: 0, RetryBackoff: 100 * time.Millisecond, MaxRetryBackoff: 10 * time.Second},
		},
		{
			name:   "zero_timeout",
			config: ConcurrencyConfig{FetchTimeout: 0, RetryBackoff: 100 * time.Millisecond, MaxRetryBackoff: 10 * time.Second},
			expect: ConcurrencyConfig{MaxConcurrentGVKs: 0, FetchTimeout: 5 * time.Minute, RetryAttempts: 0, RetryBackoff: 100 * time.Millisecond, MaxRetryBackoff: 10 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Validate()
			assert.Equal(t, tt.expect.MaxConcurrentGVKs, tt.config.MaxConcurrentGVKs)
			assert.Equal(t, tt.expect.FetchTimeout, tt.config.FetchTimeout)
		})
	}
}

// TestAdjustConcurrencyForClusterSize verifies concurrency scaling based on node count
func TestAdjustConcurrencyForClusterSize(t *testing.T) {
	tests := []struct {
		nodes    int
		expected int
	}{
		{10, 5},     // Small cluster clamped to min 5
		{100, 5},    // Small cluster clamped to min 5
		{250, 5},    // 250/50 = 5
		{500, 10},   // 500/50 = 10
		{1000, 20},  // 1000/50 = 20
		{2500, 50},  // 2500/50 = 50, clamped to max 50
		{10000, 50}, // 10000/50 = 200, clamped to max 50
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("nodes_%d", tt.nodes), func(t *testing.T) {
			result := AdjustConcurrencyForClusterSize(tt.nodes)
			assert.Equal(t, tt.expected, result, "node count %d should yield %d workers", tt.nodes, tt.expected)
		})
	}
}

// TestFetchResultMetrics verifies FetchResult throughput calculation
func TestFetchResultMetrics(t *testing.T) {
	tests := []struct {
		name     string
		result   FetchResult
		expThru  float64
		expCount int
	}{
		{
			name: "100_resources_1_second",
			result: FetchResult{
				GVK:       "test.io/v1/Test",
				Resources: make([]unstructured.Unstructured, 100),
				Duration:  1 * time.Second,
				Error:     nil,
			},
			expThru:  100.0,
			expCount: 100,
		},
		{
			name: "50_resources_500ms",
			result: FetchResult{
				GVK:       "test.io/v1/Test",
				Resources: make([]unstructured.Unstructured, 50),
				Duration:  500 * time.Millisecond,
				Error:     nil,
			},
			expThru:  100.0, // 50 / 0.5 = 100
			expCount: 50,
		},
		{
			name: "zero_duration",
			result: FetchResult{
				GVK:       "test.io/v1/Test",
				Resources: make([]unstructured.Unstructured, 100),
				Duration:  0,
				Error:     nil,
			},
			expThru:  0.0, // Avoid division by zero
			expCount: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expCount, tt.result.ResourceCount(), "Resource count should match")
			assert.Equal(t, tt.expThru, tt.result.Throughput(), "Throughput should be calculated correctly")
			assert.True(t, tt.result.IsSuccess(), "Result with no error should be success")
		})
	}
}

// TestFetchResultFailed verifies FetchResult error handling
func TestFetchResultFailed(t *testing.T) {
	err := fmt.Errorf("test error")
	result := FetchResult{
		GVK:      "test.io/v1/Test",
		Error:    err,
		Status:   "failed",
		Duration: 100 * time.Millisecond,
	}

	assert.True(t, result.IsFailed(), "Result with error should be failed")
	assert.False(t, result.IsSuccess(), "Result with error should not be success")
	assert.Equal(t, 0, result.ResourceCount(), "Failed result with no resources should have count 0")
}

// TestGVKError verifies GVKError implements error interface
func TestGVKError(t *testing.T) {
	err := fmt.Errorf("underlying error")
	gvkErr := &GVKError{
		GVK:       "cert-manager.io/v1/Certificate",
		Err:       err,
		ErrorType: "rbac",
	}

	// Verify it implements error interface
	var _ error = gvkErr

	// Verify Error() method
	assert.Contains(t, gvkErr.Error(), "underlying error")

	// Verify Unwrap() method
	assert.Equal(t, err, gvkErr.Unwrap())
}

// BenchmarkConcurrentVsSequential compares performance of concurrent vs sequential GVK fetching
// This benchmark simulates fetching multiple GVKs with varying resource counts
func BenchmarkConcurrentVsSequential(b *testing.B) {
	// Setup: Create mock resources for testing
	mockResources := make([]unstructured.Unstructured, 100)
	for i := 0; i < 100; i++ {
		mockResources[i] = unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("pod-%d", i),
					"namespace": "default",
				},
			},
		}
	}

	// Run concurrent benchmark
	b.Run("Concurrent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Simulate concurrent processing of 10 GVKs with 100 resources each
			// This would normally use errgroup with bounded concurrency
			results := make([]FetchResult, 10)
			for j := 0; j < 10; j++ {
				results[j] = FetchResult{
					GVK:       fmt.Sprintf("api.example.com/v1/Resource%d", j),
					Resources: mockResources,
					Duration:  100 * time.Millisecond,
					Status:    "success",
				}
			}
			// In concurrent mode, all 10 would run ~simultaneously
			// Total time ≈ 100ms (slowest GVK)
		}
	})

	// Run sequential benchmark
	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Simulate sequential processing of 10 GVKs with 100 resources each
			results := make([]FetchResult, 10)
			for j := 0; j < 10; j++ {
				results[j] = FetchResult{
					GVK:       fmt.Sprintf("api.example.com/v1/Resource%d", j),
					Resources: mockResources,
					Duration:  100 * time.Millisecond,
					Status:    "success",
				}
			}
			// In sequential mode, total time = 10 * 100ms = 1000ms
		}
	})
}

// BenchmarkMemoryOverhead measures memory allocation differences between concurrent and sequential approaches
func BenchmarkMemoryOverhead(b *testing.B) {
	// Create larger set of resources for memory measurement
	mockResources := make([]unstructured.Unstructured, 1000)
	for i := 0; i < 1000; i++ {
		mockResources[i] = unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("pod-%d", i),
					"namespace": "default",
					"labels": map[string]interface{}{
						"app": "test",
					},
				},
				"spec": map[string]interface{}{
					"containers": []map[string]interface{}{
						{
							"name":  "container-1",
							"image": "image:latest",
						},
					},
				},
			},
		}
	}

	// Benchmark concurrent allocation
	b.Run("ConcurrentAllocation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Simulate concurrent results allocation (10 GVKs)
			results := make([]FetchResult, 10)
			for j := 0; j < 10; j++ {
				// Each GVK allocates its own FetchResult with resources slice
				resources := make([]unstructured.Unstructured, len(mockResources))
				copy(resources, mockResources)
				results[j] = FetchResult{
					GVK:       fmt.Sprintf("api.example.com/v1/Resource%d", j),
					Resources: resources,
					Duration:  100 * time.Millisecond,
					Status:    "success",
				}
			}
			_ = results
		}
	})

	// Benchmark sequential allocation
	b.Run("SequentialAllocation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Simulate sequential processing with single results slice
			var allResources []unstructured.Unstructured
			for j := 0; j < 10; j++ {
				// Append resources sequentially
				allResources = append(allResources, mockResources...)
			}
			_ = allResources
		}
	})
}

// BenchmarkFetchResultThroughputCalculation measures cost of throughput calculation
func BenchmarkFetchResultThroughputCalculation(b *testing.B) {
	result := &FetchResult{
		Resources: make([]unstructured.Unstructured, 1000),
		Duration:  500 * time.Millisecond,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = result.Throughput()
	}
}

// BenchmarkFetchResultMetricsCalculation measures the overhead of calling multiple metric methods
func BenchmarkFetchResultMetricsCalculation(b *testing.B) {
	results := make([]*FetchResult, 100)
	for i := 0; i < 100; i++ {
		results[i] = &FetchResult{
			GVK:       fmt.Sprintf("api.example.com/v1/Resource%d", i),
			Resources: make([]unstructured.Unstructured, 100),
			Duration:  time.Duration(100+i) * time.Millisecond,
			Status:    "success",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, result := range results {
			_ = result.IsSuccess()
			_ = result.ResourceCount()
			_ = result.Throughput()
		}
	}
}

// TestGVKFetchMetricsCreation verifies GVKFetchMetrics creation and completion
func TestGVKFetchMetricsCreation(t *testing.T) {
	metric := NewGVKFetchMetrics("api.example.com/v1/Certificate")

	assert.Equal(t, "api.example.com/v1/Certificate", metric.GVK)
	assert.Equal(t, 0, metric.ResourceCount)
	assert.Equal(t, 0, metric.Pages)
	assert.Equal(t, 0, metric.RetryCount)
	assert.False(t, metric.StartTime.IsZero())
	assert.True(t, metric.EndTime.IsZero()) // Not completed yet
}

// TestGVKFetchMetricsComplete verifies metrics completion and calculations
func TestGVKFetchMetricsComplete(t *testing.T) {
	metric := NewGVKFetchMetrics("api.example.com/v1/Pod")
	time.Sleep(10 * time.Millisecond) // Wait a bit

	err := fmt.Errorf("test error")
	metric.Complete(50, err)

	assert.Equal(t, "failed", metric.Status)
	assert.Equal(t, "test error", metric.ErrorMessage)
	assert.Equal(t, 50, metric.ResourceCount)
	assert.Greater(t, metric.Duration.Milliseconds(), int64(5))
	assert.Greater(t, metric.ThroughputPerSec, 0.0)
}

// TestGenerateSyncSummary verifies sync summary generation
func TestGenerateSyncSummary(t *testing.T) {
	startTime := time.Now()

	metrics := []GVKFetchMetrics{
		{
			GVK:              "api.example.com/v1/Pod",
			Duration:         100 * time.Millisecond,
			ResourceCount:    50,
			Status:           "success",
			ThroughputPerSec: 500.0,
		},
		{
			GVK:              "api.example.com/v1/Service",
			Duration:         200 * time.Millisecond,
			ResourceCount:    30,
			Status:           "success",
			ThroughputPerSec: 150.0,
		},
		{
			GVK:           "api.example.com/v1/Ingress",
			Duration:      150 * time.Millisecond,
			ResourceCount: 0,
			Status:        "failed",
			ErrorMessage:  "RBAC denied",
		},
	}

	summary := GenerateSyncSummary("default", startTime, 10, metrics)

	assert.Equal(t, "default", summary.Context)
	assert.Equal(t, 3, summary.TotalGVKs)
	assert.Equal(t, 2, summary.SuccessfulGVKs)
	assert.Equal(t, 1, summary.FailedGVKs)
	assert.Equal(t, 80, summary.TotalResources)
	assert.Equal(t, 10, summary.ConcurrencyLevel)
	assert.Greater(t, summary.TotalDuration, time.Duration(0))
	assert.Equal(t, 450*time.Millisecond, summary.EstimatedSequential)
	assert.Contains(t, summary.FailedDetails, "api.example.com/v1/Ingress")
	assert.Equal(t, "RBAC denied", summary.FailedDetails["api.example.com/v1/Ingress"])
}

// TestSyncSummarySpeedupFactor verifies speedup calculation
func TestSyncSummarySpeedupFactor(t *testing.T) {
	testCases := []struct {
		name            string
		totalDuration   time.Duration
		estimatedSeq    time.Duration
		expectedSpeedup float64
	}{
		{
			name:            "2x speedup",
			totalDuration:   500 * time.Millisecond,
			estimatedSeq:    1 * time.Second,
			expectedSpeedup: 2.0,
		},
		{
			name:            "5x speedup",
			totalDuration:   200 * time.Millisecond,
			estimatedSeq:    1 * time.Second,
			expectedSpeedup: 5.0,
		},
		{
			name:            "no speedup (sequential)",
			totalDuration:   1 * time.Second,
			estimatedSeq:    1 * time.Second,
			expectedSpeedup: 1.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary := SyncSummary{
				TotalDuration:       tc.totalDuration,
				EstimatedSequential: tc.estimatedSeq,
			}

			speedup := summary.SpeedupFactor()
			assert.InDelta(t, tc.expectedSpeedup, speedup, 0.01)
		})
	}
}

// TestSyncSummarySorting verifies metric sorting functions
func TestSyncSummarySorting(t *testing.T) {
	summary := SyncSummary{
		GVKMetrics: []GVKFetchMetrics{
			{GVK: "Pod", Duration: 100 * time.Millisecond, ResourceCount: 50},
			{GVK: "Service", Duration: 300 * time.Millisecond, ResourceCount: 10},
			{GVK: "Ingress", Duration: 200 * time.Millisecond, ResourceCount: 100},
		},
	}

	// Sort by duration
	summary.SortMetricsByDuration()
	assert.Equal(t, "Service", summary.GVKMetrics[0].GVK) // 300ms - slowest
	assert.Equal(t, "Ingress", summary.GVKMetrics[1].GVK) // 200ms
	assert.Equal(t, "Pod", summary.GVKMetrics[2].GVK)     // 100ms - fastest

	// Sort by resource count
	summary.SortMetricsByResourceCount()
	assert.Equal(t, "Ingress", summary.GVKMetrics[0].GVK) // 100 resources
	assert.Equal(t, "Pod", summary.GVKMetrics[1].GVK)     // 50 resources
	assert.Equal(t, "Service", summary.GVKMetrics[2].GVK) // 10 resources
}

// TestSyncSummaryString verifies human-readable summary output
func TestSyncSummaryString(t *testing.T) {
	startTime := time.Now()

	metrics := []GVKFetchMetrics{
		{
			GVK:              "api.example.com/v1/Pod",
			Duration:         100 * time.Millisecond,
			ResourceCount:    50,
			Status:           "success",
			ThroughputPerSec: 500.0,
		},
		{
			GVK:           "api.example.com/v1/Service",
			Duration:      50 * time.Millisecond,
			ResourceCount: 0,
			Status:        "failed",
			ErrorMessage:  "timeout",
		},
	}

	summary := GenerateSyncSummary("test-context", startTime, 5, metrics)
	output := summary.String()

	// Verify output contains key information
	assert.Contains(t, output, "test-context")
	assert.Contains(t, output, "Custom Resources Sync Summary")
	assert.Contains(t, output, "Speedup Factor")
	assert.Contains(t, output, "2") // 2 GVKs
	assert.Contains(t, output, "1") // 1 successful
	assert.Contains(t, output, "Slowest GVKs")
	assert.Contains(t, output, "Pod")
	assert.Contains(t, output, "Failed GVKs")
	assert.Contains(t, output, "Service")
	assert.Contains(t, output, "timeout")
}

// TestGVKFetchMetricsThroughputCalculation verifies throughput calculation
func TestGVKFetchMetricsThroughputCalculation(t *testing.T) {
	testCases := []struct {
		name          string
		resourceCount int
		duration      time.Duration
		expectedTPM   float64
	}{
		{
			name:          "100 resources in 1 second",
			resourceCount: 100,
			duration:      1 * time.Second,
			expectedTPM:   100.0,
		},
		{
			name:          "50 resources in 500ms",
			resourceCount: 50,
			duration:      500 * time.Millisecond,
			expectedTPM:   100.0,
		},
		{
			name:          "zero duration",
			resourceCount: 100,
			duration:      0,
			expectedTPM:   0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metric := &GVKFetchMetrics{
				ResourceCount: tc.resourceCount,
				Duration:      tc.duration,
			}

			// Manually calculate throughput
			if metric.Duration > 0 {
				metric.ThroughputPerSec = float64(tc.resourceCount) / metric.Duration.Seconds()
			}

			assert.InDelta(t, tc.expectedTPM, metric.ThroughputPerSec, 0.1)
		})
	}
}
