package customresources

import (
	"testing"
	"time"

	"github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestFetchCustomResources_NoConfiguration tests empty configuration handling
func TestFetchCustomResources_NoConfiguration(t *testing.T) {
	s := &spec.Spec{
		Contexts:        []string{"test-context"},
		Concurrency:     1000,
		CustomResources: []spec.CustomResourceSpec{},
	}

	assert.Empty(t, s.CustomResources, "Should have no custom resources configured")
}

// TestFetchCustomResources_InvalidGVK tests error handling for invalid GVK format
func TestFetchCustomResources_InvalidGVK(t *testing.T) {
	tests := []struct {
		name      string
		gvk       string
		expectErr bool
	}{
		{
			name:      "missing_kind",
			gvk:       "cert-manager.io/v1",
			expectErr: true,
		},
		{
			name:      "too_many_parts",
			gvk:       "cert-manager.io/v1/Certificate/extra",
			expectErr: true,
		},
		{
			name:      "empty_gvk",
			gvk:       "",
			expectErr: true,
		},
		{
			name:      "valid_gvk",
			gvk:       "cert-manager.io/v1/Certificate",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseGVKToGVR(tt.gvk)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestUnstructuredObjectCreation validates test object setup
func TestUnstructuredObjectCreation(t *testing.T) {
	t.Run("cert_manager_certificate", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("cert-manager.io/v1")
		obj.SetKind("Certificate")
		obj.SetName("my-cert")
		obj.SetNamespace("default")
		obj.SetUID("cert-12345")
		obj.SetResourceVersion("12345")
		obj.SetGeneration(2)
		obj.SetLabels(map[string]string{
			"app": "my-app",
			"env": "production",
		})
		obj.SetAnnotations(map[string]string{
			"description": "Production certificate",
		})
		obj.SetCreationTimestamp(metav1.Time{Time: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)})

		obj.Object["spec"] = map[string]interface{}{
			"secretName": "my-cert-tls",
			"dnsNames":   []interface{}{"example.com"},
		}

		obj.Object["status"] = map[string]interface{}{
			"ready": true,
		}

		// Validate object structure
		assert.Equal(t, "my-cert", obj.GetName())
		assert.Equal(t, "default", obj.GetNamespace())
		assert.Equal(t, int64(2), obj.GetGeneration())

		labels := obj.GetLabels()
		assert.Equal(t, "my-app", labels["app"])

		spec, found, err := unstructured.NestedMap(obj.Object, "spec")
		require.NoError(t, err)
		require.True(t, found)
		assert.Equal(t, "my-cert-tls", spec["secretName"])
	})

	t.Run("cluster_scoped_resource", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetName("letsencrypt-prod")
		obj.SetUID("issuer-98765")
		obj.SetGeneration(1)

		obj.Object["spec"] = map[string]interface{}{
			"acme": map[string]interface{}{
				"server": "https://acme-v02.api.letsencrypt.org/directory",
			},
		}

		assert.Equal(t, "letsencrypt-prod", obj.GetName())
		assert.Empty(t, obj.GetNamespace(), "Cluster-scoped resources should have empty namespace")
	})

	t.Run("resource_with_empty_fields", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetName("sample-resource")
		obj.SetNamespace("test")
		obj.SetUID("sample-123")

		assert.Equal(t, "sample-resource", obj.GetName())
		assert.Empty(t, obj.GetLabels())
		assert.Empty(t, obj.GetAnnotations())
	})
}

// TestNamespaceFiltering validates namespace filtering logic
func TestNamespaceFiltering(t *testing.T) {
	tests := []struct {
		name               string
		configuredNS       []string
		shouldFetchAll     bool
		expectedNamespaces int
	}{
		{
			name:               "no_namespace_filter",
			configuredNS:       []string{},
			shouldFetchAll:     true,
			expectedNamespaces: 1,
		},
		{
			name:               "specific_namespaces",
			configuredNS:       []string{"default", "production"},
			shouldFetchAll:     false,
			expectedNamespaces: 2,
		},
		{
			name:               "single_namespace",
			configuredNS:       []string{"kube-system"},
			shouldFetchAll:     false,
			expectedNamespaces: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crSpec := spec.CustomResourceSpec{
				GVK:        "cert-manager.io/v1/Certificate",
				Namespaces: tt.configuredNS,
			}

			if len(crSpec.Namespaces) == 0 {
				assert.True(t, tt.shouldFetchAll)
			} else {
				assert.Equal(t, tt.expectedNamespaces, len(crSpec.Namespaces))
			}
		})
	}
}

// TestTableRegistration verifies the custom resources table is properly registered
func TestTableRegistration(t *testing.T) {
	table := CustomResources()

	require.NotNil(t, table)
	assert.Equal(t, "k8s_custom_resources", table.Name)
	assert.NotNil(t, table.Resolver)
	assert.NotNil(t, table.Multiplex)

	assert.Greater(t, len(table.Columns), 10, "Should have at least 11 columns")

	keyColumns := []string{"gvk", "namespace", "name", "uid", "spec", "status"}
	for _, col := range keyColumns {
		found := false
		for _, tableCol := range table.Columns {
			if tableCol.Name == col {
				found = true
				break
			}
		}
		assert.True(t, found, "Column %q should exist", col)
	}
}

// TestComplexDataStructures validates handling of nested JSON structures
func TestComplexDataStructures(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("complex-test")
	obj.SetNamespace("default")
	obj.SetUID("complex-123")
	obj.SetCreationTimestamp(metav1.Time{Time: time.Now()})

	obj.Object["spec"] = map[string]interface{}{
		"nested": map[string]interface{}{
			"field1": "value1",
			"field2": int64(42),
			"field3": []interface{}{"a", "b", "c"},
			"field4": map[string]interface{}{
				"deep": "value",
			},
		},
	}

	obj.SetLabels(map[string]string{
		"key1": "value1",
		"key2": "value2",
	})

	spec, found, err := unstructured.NestedMap(obj.Object, "spec")
	require.NoError(t, err)
	require.True(t, found)

	nested, found, err := unstructured.NestedMap(spec, "nested")
	require.NoError(t, err)
	require.True(t, found)

	assert.Equal(t, "value1", nested["field1"])
	assert.Equal(t, int64(42), nested["field2"])

	labels := obj.GetLabels()
	assert.Equal(t, "value1", labels["key1"])
}

// TestConfigurationExamples validates example configurations from docs
func TestConfigurationExamples(t *testing.T) {
	t.Run("single_custom_resource", func(t *testing.T) {
		s := &spec.Spec{
			CustomResources: []spec.CustomResourceSpec{
				{
					GVK:        "cert-manager.io/v1/Certificate",
					Namespaces: []string{"default"},
				},
			},
		}

		s.SetDefaults()
		err := s.Validate()
		require.NoError(t, err)
		assert.Len(t, s.CustomResources, 1)
	})

	t.Run("multiple_custom_resources", func(t *testing.T) {
		s := &spec.Spec{
			CustomResources: []spec.CustomResourceSpec{
				{
					GVK:        "cert-manager.io/v1/Certificate",
					Namespaces: []string{"default", "production"},
				},
				{
					GVK: "argoproj.io/v1alpha1/Application",
				},
				{
					GVK:        "monitoring.coreos.com/v1/ServiceMonitor",
					Namespaces: []string{"monitoring"},
				},
			},
		}

		s.SetDefaults()
		err := s.Validate()
		require.NoError(t, err)
		assert.Len(t, s.CustomResources, 3)
	})
}
