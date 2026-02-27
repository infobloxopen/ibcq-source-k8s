package customresources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/apache/arrow/go/v16/arrow"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
	"k8s.io/client-go/dynamic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1schema "k8s.io/apimachinery/pkg/runtime/schema"
)

// CustomResources returns the table definition for k8s_custom_resources
func CustomResources() *schema.Table {
	return &schema.Table{
		Name:      "k8s_custom_resources",
		Resolver:  fetchCustomResources,
		Multiplex: client.ContextMultiplex,
		Transform: transformers.TransformWithStruct(&CustomResourceRow{},
			transformers.WithPrimaryKeys("context", "gvk", "namespace", "name"),
		),
		Columns: schema.ColumnList{
			client.ContextColumn,
			{
				Name:        "gvk",
				Type:        arrow.BinaryTypes.String,
				Description: "GroupVersionKind in format 'group/version/kind'",
				Resolver:    schema.PathResolver("GVK"),
			},
			{
				Name:        "namespace",
				Type:        arrow.BinaryTypes.String,
				Description: "Namespace of the resource. Empty for cluster-scoped resources.",
				Resolver:    schema.PathResolver("Namespace"),
			},
			{
				Name:        "name",
				Type:        arrow.BinaryTypes.String,
				Description: "Name of the resource",
				Resolver:    schema.PathResolver("Name"),
			},
			{
				Name:        "uid",
				Type:        arrow.BinaryTypes.String,
				Description: "Unique identifier for the resource",
				Resolver:    schema.PathResolver("UID"),
			},
			{
				Name:        "resource_version",
				Type:        arrow.BinaryTypes.String,
				Description: "Resource version for optimistic concurrency",
				Resolver:    schema.PathResolver("ResourceVersion"),
			},
			{
				Name:        "generation",
				Type:        arrow.PrimitiveTypes.Int64,
				Description: "Generation number for spec updates",
				Resolver:    schema.PathResolver("Generation"),
			},
			{
				Name:        "labels",
				Type:        arrow.BinaryTypes.String,
				Description: "Labels as JSON string",
				Resolver:    schema.PathResolver("Labels"),
			},
			{
				Name:        "annotations",
				Type:        arrow.BinaryTypes.String,
				Description: "Annotations as JSON string",
				Resolver:    schema.PathResolver("Annotations"),
			},
			{
				Name:        "created_at",
				Type:        arrow.FixedWidthTypes.Timestamp_us,
				Description: "Creation timestamp",
				Resolver:    schema.PathResolver("CreatedAt"),
			},
			{
				Name:        "spec",
				Type:        arrow.BinaryTypes.String,
				Description: "Resource spec as JSON string",
				Resolver:    schema.PathResolver("Spec"),
			},
			{
				Name:        "status",
				Type:        arrow.BinaryTypes.String,
				Description: "Resource status as JSON string",
				Resolver:    schema.PathResolver("Status"),
			},
		},
	}
}

// CustomResourceRow represents a single custom resource in the table
type CustomResourceRow struct {
	Context         string `json:"context"`
	GVK             string `json:"gvk"`
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resource_version"`
	Generation      int64  `json:"generation"`
	Labels          string `json:"labels"`           // JSON-encoded map
	Annotations     string `json:"annotations"`      // JSON-encoded map
	CreatedAt       string `json:"created_at"`       // ISO 8601 timestamp
	Spec            string `json:"spec"`             // JSON-encoded spec
	Status          string `json:"status,omitempty"` // JSON-encoded status
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
		Context:         cl.Context,
		GVK:             gvk,
		Namespace:       obj.GetNamespace(),
		Name:            obj.GetName(),
		UID:             string(obj.GetUID()),
		ResourceVersion: obj.GetResourceVersion(),
		Generation:      obj.GetGeneration(),
	}

	// Convert labels to JSON
	if labels := obj.GetLabels(); labels != nil {
		labelsJSON, err := json.Marshal(labels)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels: %w", err)
		}
		row.Labels = string(labelsJSON)
	} else {
		row.Labels = "{}"
	}

	// Convert annotations to JSON
	if annotations := obj.GetAnnotations(); annotations != nil {
		annotationsJSON, err := json.Marshal(annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal annotations: %w", err)
		}
		row.Annotations = string(annotationsJSON)
	} else {
		row.Annotations = "{}"
	}

	// Format creation timestamp
	if creationTime := obj.GetCreationTimestamp(); !creationTime.IsZero() {
		row.CreatedAt = creationTime.Format(time.RFC3339)
	}

	// Extract spec
	if spec, found, err := unstructured.NestedMap(obj.Object, "spec"); err != nil {
		return nil, fmt.Errorf("failed to extract spec: %w", err)
	} else if found && spec != nil {
		specJSON, err := json.Marshal(spec)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal spec: %w", err)
		}
		row.Spec = string(specJSON)
	} else {
		row.Spec = "{}"
	}

	// Extract status
	if status, found, err := unstructured.NestedMap(obj.Object, "status"); err != nil {
		return nil, fmt.Errorf("failed to extract status: %w", err)
	} else if found && status != nil {
		statusJSON, err := json.Marshal(status)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal status: %w", err)
		}
		row.Status = string(statusJSON)
	} else {
		row.Status = "{}"
	}

	return row, nil
}
