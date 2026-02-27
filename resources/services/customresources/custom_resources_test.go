package customresources

import (
	"testing"

	"github.com/apache/arrow/go/v16/arrow"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Verify required columns exist
	requiredColumns := []string{
		"context",
		"gvk",
		"namespace",
		"name",
		"uid",
		"resource_version",
		"generation",
		"labels",
		"annotations",
		"created_at",
		"spec",
		"status",
	}

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

	tests := []struct {
		columnName   string
		expectedType arrow.DataType
	}{
		{"gvk", arrow.BinaryTypes.String},
		{"namespace", arrow.BinaryTypes.String},
		{"name", arrow.BinaryTypes.String},
		{"uid", arrow.BinaryTypes.String},
		{"generation", arrow.PrimitiveTypes.Int64},
		{"created_at", arrow.FixedWidthTypes.Timestamp_us},
	}

	for _, tt := range tests {
		t.Run(tt.columnName, func(t *testing.T) {
			var col *schema.Column
			for i := range table.Columns {
				if table.Columns[i].Name == tt.columnName {
					col = &table.Columns[i]
					break
				}
			}
			require.NotNil(t, col, "Column %q not found", tt.columnName)
			assert.Equal(t, tt.expectedType, col.Type, "Column %q should have type %v", tt.columnName, tt.expectedType)
		})
	}
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
		Context:         "test-context",
		GVK:             "cert-manager.io/v1/Certificate",
		Namespace:       "default",
		Name:            "test-cert",
		UID:             "12345",
		ResourceVersion: "v1",
		Generation:      1,
		Labels:          `{"app":"test"}`,
		Annotations:     `{"description":"test"}`,
		CreatedAt:       "2024-01-01T00:00:00Z",
		Spec:            `{"secretName":"test-secret"}`,
		Status:          `{"ready":true}`,
	}

	// Verify fields are accessible
	assert.Equal(t, "test-context", row.Context)
	assert.Equal(t, "cert-manager.io/v1/Certificate", row.GVK)
	assert.Equal(t, "default", row.Namespace)
	assert.Equal(t, "test-cert", row.Name)
	assert.Equal(t, int64(1), row.Generation)
}

func TestCustomResources_PrimaryKeys(t *testing.T) {
	table := CustomResources()

	// Verify table has Transform configured
	assert.NotNil(t, table.Transform)

	// The primary keys are: context, gvk, namespace, name
	// This test verifies the table is properly configured for these keys
	// Actual validation happens during TransformTables in the plugin
}
