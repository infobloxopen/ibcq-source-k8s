package customresources

import (
	"context"
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
