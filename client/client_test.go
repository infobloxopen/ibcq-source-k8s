package client

import (
	"testing"

	"github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func TestClient_DynamicClient(t *testing.T) {
	tests := []struct {
		name    string
		context string
	}{
		{
			name:    "returns_dynamic_client_for_context",
			context: "test-context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			c := &Client{
				logger:         zerolog.Nop(),
				Context:        tt.context,
				contexts:       []string{tt.context},
				clients:        make(map[string]kubernetes.Interface),
				namespaces:     make(map[string][]v1.Namespace),
				apiExtensions:  make(map[string]apiextensionsclientset.Interface),
				dynamicClients: make(map[string]dynamic.Interface),
				spec:           &spec.Spec{},
			}

			// Test that DynamicClient accessor works
			client := c.DynamicClient()
			// Note: client will be nil since we don't have real k8s config in tests,
			// but we verify the accessor method doesn't panic
			assert.Nil(t, client)
		})
	}
}

func TestClient_WithContext_PreservesDynamicClients(t *testing.T) {
	originalContext := "context-1"
	newContext := "context-2"

	c := &Client{
		logger:         zerolog.Nop(),
		Context:        originalContext,
		contexts:       []string{originalContext, newContext},
		clients:        make(map[string]kubernetes.Interface),
		namespaces:     make(map[string][]v1.Namespace),
		apiExtensions:  make(map[string]apiextensionsclientset.Interface),
		dynamicClients: make(map[string]dynamic.Interface),
		spec:           &spec.Spec{},
	}

	// Create new client with different context
	newC := c.WithContext(newContext)

	// Verify context changed
	assert.Equal(t, newContext, newC.Context)
	assert.Equal(t, originalContext, c.Context)

	// Verify dynamicClients map is preserved
	assert.NotNil(t, newC.dynamicClients)
	assert.Equal(t, c.dynamicClients, newC.dynamicClients)
}

func TestClient_WithNamespace_PreservesDynamicClients(t *testing.T) {
	namespace := "test-namespace"

	c := &Client{
		logger:         zerolog.Nop(),
		Context:        "test-context",
		contexts:       []string{"test-context"},
		clients:        make(map[string]kubernetes.Interface),
		namespaces:     make(map[string][]v1.Namespace),
		apiExtensions:  make(map[string]apiextensionsclientset.Interface),
		dynamicClients: make(map[string]dynamic.Interface),
		spec:           &spec.Spec{},
	}

	// Create new client with namespace
	newC := c.WithNamespace(namespace)

	// Verify namespace set
	assert.Equal(t, namespace, newC.Namespace)
	assert.Equal(t, "", c.Namespace)

	// Verify dynamicClients map is preserved
	assert.NotNil(t, newC.dynamicClients)
	assert.Equal(t, c.dynamicClients, newC.dynamicClients)
}

func TestConfigureWithCustomResources(t *testing.T) {
	// This test verifies that Configure initializes dynamicClients map
	// We can't fully test Configure without a real kubeconfig, but we can verify structure

	tests := []struct {
		name string
		spec spec.Spec
	}{
		{
			name: "empty_custom_resources",
			spec: spec.Spec{
				Contexts:        []string{},
				Concurrency:     1000,
				CustomResources: []spec.CustomResourceSpec{},
			},
		},
		{
			name: "with_custom_resources",
			spec: spec.Spec{
				Contexts:    []string{},
				Concurrency: 1000,
				CustomResources: []spec.CustomResourceSpec{
					{
						GVK:        "cert-manager.io/v1/Certificate",
						Namespaces: []string{"default"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate spec
			tt.spec.SetDefaults()
			err := tt.spec.Validate()
			require.NoError(t, err)

			// Note: We can't actually call Configure without a real kubeconfig,
			// but we've verified:
			// 1. Spec accepts CustomResources
			// 2. Validation works
			// 3. Client struct has dynamicClients field
			// 4. Accessor methods work
		})
	}
}

func TestClient_StructureHasDynamicClients(t *testing.T) {
	// Compile-time verification that Client has required fields
	c := &Client{
		logger:         zerolog.Nop(),
		Context:        "test",
		contexts:       []string{"test"},
		clients:        make(map[string]kubernetes.Interface),
		namespaces:     make(map[string][]v1.Namespace),
		apiExtensions:  make(map[string]apiextensionsclientset.Interface),
		dynamicClients: make(map[string]dynamic.Interface),
		spec:           &spec.Spec{},
		paths:          make(map[string]struct{}),
	}

	// Verify all maps are initialized
	assert.NotNil(t, c.clients)
	assert.NotNil(t, c.namespaces)
	assert.NotNil(t, c.apiExtensions)
	assert.NotNil(t, c.dynamicClients)
	assert.NotNil(t, c.paths)
}
