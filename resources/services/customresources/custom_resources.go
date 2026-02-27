package customresources

import (
	"context"
	"fmt"
	"strings"

	"github.com/apache/arrow/go/v16/arrow"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client"
	"github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec"
	"github.com/cloudquery/plugin-sdk/v4/schema"
	"github.com/cloudquery/plugin-sdk/v4/transformers"
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
	
	// TODO: Phase 4 - Implementation will fetch resources from dynamic client
	// For now, this is a placeholder that returns nil (no resources)
	// The fetcher logic will iterate through spec.CustomResources and fetch each GVK
	
	_ = cl
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
// Placeholder for Phase 4 implementation
func fetchResourcesByGVR(ctx context.Context, cl *client.Client, gvr metav1schema.GroupVersionResource, crSpec spec.CustomResourceSpec, res chan<- any) error {
	// TODO: Phase 4 - Implement actual fetching logic
	// Will use cl.DynamicClient() to list resources
	// Will handle pagination with continue tokens
	// Will filter by namespaces if specified in crSpec
	
	_ = ctx
	_ = cl
	_ = gvr
	_ = crSpec
	_ = res
	return nil
}

// convertToCustomResourceRow converts an unstructured object to CustomResourceRow
// Placeholder for Phase 4 implementation
func convertToCustomResourceRow(ctx context.Context, cl *client.Client, gvk string, obj *unstructured.Unstructured) (*CustomResourceRow, error) {
	// TODO: Phase 4 - Implement conversion logic
	// Will extract metadata (name, namespace, uid, etc.)
	// Will JSON-encode spec and status
	// Will format timestamps properly
	
	_ = ctx
	_ = cl
	_ = gvk
	_ = obj
	return nil, nil
}
