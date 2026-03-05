package customresources

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudquery/cloudquery/plugins/source/k8s/client"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
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

// fetchCustomResources retrieves custom resources based on plugin configuration
func fetchCustomResources(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource, res chan<- any) error {
	cl := meta.(*client.Client)

	// Get custom resources from configuration
	spec := cl.Spec()
	if spec == nil || len(spec.CustomResources) == 0 {
		// No custom resources configured, skip
		return nil
	}

	// Iterate through each configured custom resource
	for _, crSpec := range spec.CustomResources {
		// Parse GVK to GVR
		gvr, err := parseGVKToGVR(crSpec.GVK)
		if err != nil {
			return fmt.Errorf("invalid GVK %q: %w", crSpec.GVK, err)
		}

		// Fetch resources for this GVK
		if err := fetchResourcesByGVR(ctx, cl, crSpec.GVK, gvr, crSpec, res); err != nil {
			return fmt.Errorf("failed to fetch resources for %q: %w", crSpec.GVK, err)
		}
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

// fetchResourcesByGVR fetches all resources for a given GVR
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
