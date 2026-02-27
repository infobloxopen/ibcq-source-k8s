# Implementation Plan: Custom Resources Support

**Feature**: Add Custom Resources (CR) collection to K8s CloudQuery plugin  
**Branch**: `cq-crd`  
**Created**: 2026-02-27  
**Status**: Draft

## Executive Summary

This plan outlines the implementation of custom resource collection functionality for the K8s CloudQuery plugin. The implementation will enable users to selectively collect Kubernetes Custom Resources (CRs) and store them in a unified `k8s_custom_resources` table, compatible with CloudQuery's closed-source plugin schema.

## Technical Context

### Current Architecture
- **Plugin Type**: CloudQuery source plugin (Golang)
- **SDK**: CloudQuery Plugin SDK v4
- **K8s Client**: Official client-go library
- **Data Format**: Apache Arrow records
- **Table Pattern**: Each K8s resource type has dedicated table definition in `resources/services/<api-group>/`

### Existing Patterns to Follow
1. **Client Structure**: `client/client.go` provides base `Client` struct with context-based K8s clients
2. **Table Definition**: Each resource has `*schema.Table` with Name, Resolver, Multiplex, Transform, Columns
3. **Fetch Pattern**: Resolvers use `metav1.ListOptions` with pagination (Continue token)
4. **Testing**: Mock-based tests using gomock for K8s client interfaces
5. **Documentation**: Each table has corresponding markdown doc in `docs/tables/`

### Key Constraints from Constitution
- ✅ Test-driven development (tests before implementation)
- ✅ Use official Kubernetes client-go library
- ✅ Apache Arrow format via CloudQuery SDK transformers
- ✅ Mock-based testing with mockgen
- ✅ Follow existing code organization patterns

## Architecture & Design

### 1. Configuration Schema Extension

**Location**: `client/spec/spec.go`

Add new configuration field to `Spec` struct:

```go
type Spec struct {
    Contexts    []string `yaml:"contexts,omitempty" json:"contexts"`
    Concurrency int      `yaml:"concurrency,omitempty" json:"concurrency"`
    
    // New field for custom resources
    CustomResources []CustomResourceSpec `yaml:"custom_resources,omitempty" json:"custom_resources"`
}

type CustomResourceSpec struct {
    // GroupVersionKind in format "group/version/kind" 
    // Examples: "cert-manager.io/v1/Certificate", "apps/v1/Deployment"
    GVK string `yaml:"gvk" json:"gvk" jsonschema:"required,pattern=^[a-z0-9.-]+/[a-z0-9]+/[A-Z][a-zA-Z0-9]*$"`
    
    // Optional: Namespace filter (empty = all namespaces)
    Namespaces []string `yaml:"namespaces,omitempty" json:"namespaces"`
}
```

**Why**: Follows existing `Spec` pattern, uses YAML for configuration consistency, validates GVK format at parse time.

### 2. Dynamic Client Integration

**Location**: `client/client.go`

Extend `Client` struct to include dynamic client:

```go
import "k8s.io/client-go/dynamic"

type Client struct {
    logger        zerolog.Logger
    clients       map[string]kubernetes.Interface
    namespaces    map[string][]v1.Namespace
    apiExtensions map[string]apiextensionsclientset.Interface
    
    // New: Dynamic client for custom resources
    dynamicClients map[string]dynamic.Interface  // context -> dynamic client
    
    spec          *spec.Spec
    contexts      []string
    paths         map[string]struct{}
    Context       string
    Namespace     string
}

func (c *Client) DynamicClient() dynamic.Interface {
    return c.dynamicClients[c.Context]
}
```

Initialize in `Configure()`:

```go
func Configure(ctx context.Context, logger zerolog.Logger, s spec.Spec) (schema.ClientMeta, error) {
    // ... existing code ...
    
    // Initialize dynamic clients
    dynamicClients := make(map[string]dynamic.Interface)
    for ctxName, restConfig := range restConfigs {
        dynClient, err := dynamic.NewForConfig(restConfig)
        if err != nil {
            return nil, fmt.Errorf("failed to create dynamic client for context %s: %w", ctxName, err)
        }
        dynamicClients[ctxName] = dynClient
    }
    
    client := &Client{
        // ... existing fields ...
        dynamicClients: dynamicClients,
    }
    
    return client, nil
}
```

**Why**: Dynamic client is required for fetching arbitrary custom resources. One client per context maintains existing multi-context pattern.

### 3. Custom Resources Table Definition

**Location**: `resources/services/customresources/custom_resources.go` (new package)

```go
package customresources

import (
    "context"
    "encoding/json"
    
    "github.com/apache/arrow/go/v16/arrow"
    "github.com/cloudquery/cloudquery/plugins/source/k8s/client"
    "github.com/cloudquery/plugin-sdk/v4/schema"
    "github.com/cloudquery/plugin-sdk/v4/transformers"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CustomResources() *schema.Table {
    return &schema.Table{
        Name:      "k8s_custom_resources",
        Resolver:  fetchCustomResources,
        Multiplex: customResourceMultiplex,
        Transform: transformers.TransformWithStruct(&CustomResourceRow{}, transformers.WithPrimaryKeys("UID")),
        Columns: schema.ColumnList{
            client.ContextColumn,
            {
                Name:        "api_version",
                Type:        arrow.BinaryTypes.String,
                Resolver:    schema.PathResolver("APIVersion"),
                Description: "API version of the custom resource (e.g., cert-manager.io/v1)",
            },
            {
                Name:        "kind",
                Type:        arrow.BinaryTypes.String,
                Resolver:    schema.PathResolver("Kind"),
                Description: "Kind of the custom resource (e.g., Certificate)",
            },
            {
                Name:        "name",
                Type:        arrow.BinaryTypes.String,
                Resolver:    schema.PathResolver("Metadata.Name"),
            },
            {
                Name:        "namespace",
                Type:        arrow.BinaryTypes.String,
                Resolver:    schema.PathResolver("Metadata.Namespace"),
            },
            {
                Name:        "uid",
                Type:        arrow.BinaryTypes.String,
                Resolver:    schema.PathResolver("Metadata.UID"),
                PrimaryKey:  true,
            },
            {
                Name:        "labels",
                Type:        arrow.BinaryTypes.String, // JSON encoded
                Resolver:    jsonMapResolver("Metadata.Labels"),
            },
            {
                Name:        "annotations",
                Type:        arrow.BinaryTypes.String, // JSON encoded
                Resolver:    jsonMapResolver("Metadata.Annotations"),
            },
            {
                Name:        "owner_references",
                Type:        arrow.BinaryTypes.String, // JSON encoded
                Resolver:    jsonResolver("Metadata.OwnerReferences"),
            },
            {
                Name:        "finalizers",
                Type:        arrow.ListOf(arrow.BinaryTypes.String),
                Resolver:    schema.PathResolver("Metadata.Finalizers"),
            },
            {
                Name:        "spec",
                Type:        arrow.BinaryTypes.String, // JSON encoded
                Resolver:    jsonResolver("Spec"),
                Description: "Custom resource specification as JSON",
            },
            {
                Name:        "status",
                Type:        arrow.BinaryTypes.String, // JSON encoded  
                Resolver:    jsonResolver("Status"),
                Description: "Custom resource status as JSON",
            },
            {
                Name:        "creation_timestamp",
                Type:        arrow.FixedWidthTypes.Timestamp_us,
                Resolver:    schema.PathResolver("Metadata.CreationTimestamp"),
            },
        },
    }
}

type CustomResourceRow struct {
    APIVersion string
    Kind       string
    Metadata   struct {
        Name              string
        Namespace         string
        UID               string
        Labels            map[string]string
        Annotations       map[string]string
        OwnerReferences   []metav1.OwnerReference
        Finalizers        []string
        CreationTimestamp metav1.Time
    }
    Spec   map[string]interface{}
    Status map[string]interface{}
}
```

**Why**: 
- Schema includes all required columns for custom resources
- Uses `unstructured.Unstructured` internally since CR schemas are dynamic
- JSON encoding for spec/status allows storing arbitrary nested structures
- UID as primary key follows K8s resource pattern
- CloudQuery SDK automatically adds _cq_id and _cq_parent_id columns via transformers

### 4. Custom Resource Fetcher

**Location**: `resources/services/customresources/custom_resources.go`

```go
func customResourceMultiplex(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource) []schema.ClientMeta {
    cl := meta.(*client.Client)
    
    // Get configured custom resources from spec
    crSpecs := cl.Spec().CustomResources
    if len(crSpecs) == 0 {
        return []schema.ClientMeta{} // No CRs configured, skip
    }
    
    var result []schema.ClientMeta
    
    // Multiplex by context, then by GVK
    for _, ctxName := range cl.Contexts() {
        ctxClient := cl.WithContext(ctxName)
        for _, crSpec := range crSpecs {
            result = append(result, &customResourceClient{
                Client: ctxClient,
                gvk:    parseGVK(crSpec.GVK),
                nsFilter: crSpec.Namespaces,
            })
        }
    }
    
    return result
}

type customResourceClient struct {
    *client.Client
    gvk      schema.GroupVersionKind
    nsFilter []string // empty = all namespaces
}

func fetchCustomResources(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource, res chan<- any) error {
    crClient := meta.(*customResourceClient)
    dynClient := crClient.DynamicClient()
    
    gvr := schema.GroupVersionResource{
        Group:    crClient.gvk.Group,
        Version:  crClient.gvk.Version,
        Resource: pluralize(crClient.gvk.Kind), // e.g., Certificate -> certificates
    }
    
    // Determine namespaces to query
    namespaces := crClient.nsFilter
    if len(namespaces) == 0 {
        // Fetch from all namespaces
        nsList := crClient.Namespaces()
        namespaces = make([]string, len(nsList))
        for i, ns := range nsList {
            namespaces[i] = ns.Name
        }
        // Also include cluster-scoped resources
        namespaces = append(namespaces, "") // empty string = cluster-scoped
    }
    
    for _, ns := range namespaces {
        var resourceClient dynamic.ResourceInterface
        if ns == "" {
            // Cluster-scoped resource
            resourceClient = dynClient.Resource(gvr)
        } else {
            resourceClient = dynClient.Resource(gvr).Namespace(ns)
        }
        
        opts := metav1.ListOptions{}
        for {
            result, err := resourceClient.List(ctx, opts)
            if err != nil {
                // Log error but continue with other resources
                crClient.Logger().Err(err).
                    Str("gvr", gvr.String()).
                    Str("namespace", ns).
                    Msg("failed to list custom resources")
                break
            }
            
            // Convert unstructured to CustomResourceRow
            for _, item := range result.Items {
                row := convertUnstructuredToRow(&item)
                res <- row
            }
            
            if result.GetContinue() == "" {
                break
            }
            opts.Continue = result.GetContinue()
        }
    }
    
    return nil
}

func convertUnstructuredToRow(u *unstructured.Unstructured) *CustomResourceRow {
    row := &CustomResourceRow{
        APIVersion: u.GetAPIVersion(),
        Kind:       u.GetKind(),
    }
    
    // Metadata
    row.Metadata.Name = u.GetName()
    row.Metadata.Namespace = u.GetNamespace()
    row.Metadata.UID = string(u.GetUID())
    row.Metadata.Labels = u.GetLabels()
    row.Metadata.Annotations = u.GetAnnotations()
    row.Metadata.OwnerReferences = u.GetOwnerReferences()
    row.Metadata.Finalizers = u.GetFinalizers()
    row.Metadata.CreationTimestamp = u.GetCreationTimestamp()
    
    // Spec and Status
    if spec, found := u.Object["spec"]; found {
        if specMap, ok := spec.(map[string]interface{}); ok {
            row.Spec = specMap
        }
    }
    
    if status, found := u.Object["status"]; found {
        if statusMap, ok := status.(map[string]interface{}); ok {
            row.Status = statusMap
        }
    }
    
    return row
}

func parseGVK(gvkStr string) schema.GroupVersionKind {
    // Parse "group/version/kind" format
    parts := strings.Split(gvkStr, "/")
    return schema.GroupVersionKind{
        Group:   parts[0],
        Version: parts[1],
        Kind:    parts[2],
    }
}

func pluralize(kind string) string {
    // Simple pluralization (can be enhanced)
    // TODO: Use proper pluralization logic or fetch from CRD
    lower := strings.ToLower(kind)
    if strings.HasSuffix(lower, "s") {
        return lower + "es"
    }
    if strings.HasSuffix(lower, "y") {
        return lower[:len(lower)-1] + "ies"
    }
    return lower + "s"
}
```

**Why**:
- Multiplex pattern allows fetching different GVKs in parallel
- Pagination via Continue token handles large CR sets
- Error handling per GVK allows partial success
- Namespace filtering reduces data transfer

### 5. Helper Functions

**Location**: `resources/services/customresources/helpers.go`

```go
package customresources

import (
    "encoding/json"
    "github.com/cloudquery/plugin-sdk/v4/schema"
)

func jsonResolver(path string) schema.ColumnResolver {
    return func(ctx context.Context, meta schema.ClientMeta, resource *schema.Resource, c schema.Column) error {
        data := resource.GetItem().(*CustomResourceRow)
        
        var value interface{}
        // Use path resolver to get value
        // ... path traversal logic ...
        
        if value == nil {
            return resource.Set(c.Name, nil)
        }
        
        jsonBytes, err := json.Marshal(value)
        if err != nil {
            return err
        }
        
        return resource.Set(c.Name, string(jsonBytes))
    }
}

func jsonMapResolver(path string) schema.ColumnResolver {
    return func(ctx context.Context, meta schema.ClientMeta, resource *schema.Resource, c schema.Column) error {
        data := resource.GetItem().(*CustomResourceRow)
        
        var mapValue map[string]string
        // ... path traversal to get map ...
        
        if mapValue == nil || len(mapValue) == 0 {
            return resource.Set(c.Name, nil)
        }
        
        jsonBytes, err := json.Marshal(mapValue)
        if err != nil {
            return err
        }
        
        return resource.Set(c.Name, string(jsonBytes))
    }
}
```

### 6. Register Table in Plugin

**Location**: `resources/plugin/plugin.go`

Add import and table registration:

```go
import (
    "github.com/cloudquery/cloudquery/plugins/source/k8s/resources/services/customresources"
)

func getTables() schema.Tables {
    return schema.Tables{
        // ... existing tables ...
        customresources.CustomResources(), // Add this
    }
}
```

### 7. Configuration Validation

**Location**: `client/spec/spec.go`

Add validation method:

```go
func (s *Spec) Validate() error {
    var errs []error
    
    for i, cr := range s.CustomResources {
        if err := validateGVK(cr.GVK); err != nil {
            errs = append(errs, fmt.Errorf("custom_resources[%d].gvk: %w", i, err))
        }
    }
    
    if len(errs) > 0 {
        return errors.Join(errs...)
    }
    
    return nil
}

func validateGVK(gvk string) error {
    parts := strings.Split(gvk, "/")
    if len(parts) != 3 {
        return fmt.Errorf("invalid GVK format %q: must be group/version/kind", gvk)
    }
    
    group, version, kind := parts[0], parts[1], parts[2]
    
    if group == "" {
        return fmt.Errorf("group cannot be empty")
    }
    if version == "" {
        return fmt.Errorf("version cannot be empty")
    }
    if kind == "" || !unicode.IsUpper(rune(kind[0])) {
        return fmt.Errorf("kind must start with uppercase letter")
    }
    
    return nil
}
```

Call validation in `Configure()`.

## Testing Strategy

### 1. Unit Tests

**Location**: `resources/services/customresources/custom_resources_test.go`

```go
func TestCustomResources(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    
    // Create mock dynamic client
    mockDynamic := createMockDynamicClient(t, ctrl)
    
    // Configure test client
    c := client.TestClient{
        DynamicClients: map[string]dynamic.Interface{
            "test-context": mockDynamic,
        },
        Spec: &spec.Spec{
            CustomResources: []spec.CustomResourceSpec{
                {GVK: "cert-manager.io/v1/Certificate"},
            },
        },
    }
    
    // Execute table fetch
    table := CustomResources()
    // ... test assertions ...
}

func createMockDynamicClient(t *testing.T, ctrl *gomock.Controller) dynamic.Interface {
    // Create mock with expected CR data
    mockResource := &unstructured.UnstructuredList{
        Items: []unstructured.Unstructured{
            {
                Object: map[string]interface{}{
                    "apiVersion": "cert-manager.io/v1",
                    "kind":       "Certificate",
                    "metadata": map[string]interface{}{
                        "name":      "test-cert",
                        "namespace": "default",
                        "uid":       "test-uid-123",
                    },
                    "spec": map[string]interface{}{
                        "secretName": "test-cert-tls",
                        "dnsNames":   []interface{}{"example.com"},
                    },
                },
            },
        },
    }
    
    // Setup mock expectations
    // ... gomock setup ...
    
    return mockDynamic
}
```

### 2. Integration Tests

**Location**: `resources/services/customresources/integration_test.go`

```go
// +build integration

func TestCustomResourcesIntegration(t *testing.T) {
    // Requires actual K8s cluster with CRDs
    // Use kind or minikube for CI
}
```

### 3. Mock Generation

Update `mockgen.go`:

```go
//go:generate mockgen -package=mocks -destination=./mocks/dynamic/dynamic.go k8s.io/client-go/dynamic Interface,ResourceInterface,NamespaceableResourceInterface
```

## Documentation Updates

### 1. Table Documentation

**Location**: `docs/tables/k8s_custom_resources.md`

```markdown
# k8s_custom_resources

This table contains Kubernetes Custom Resources (CRs) as configured in the plugin specification.

## Columns

| Name | Type | Description |
|------|------|-------------|
| context | String | Kubernetes context name |
| api_version | String | API version (e.g., cert-manager.io/v1) |
| kind | String | Resource kind (e.g., Certificate) |
| name | String | Resource name |
| namespace | String | Namespace (empty for cluster-scoped) |
| uid | String | Unique identifier (primary key) |
| labels | JSON | Resource labels |
| annotations | JSON | Resource annotations |
| owner_references | JSON | Owner references |
| finalizers | List<String> | Finalizers |
| spec | JSON | Custom resource specification |
| status | JSON | Custom resource status |
| creation_timestamp | Timestamp | Creation time |

## Configuration

```yaml
kind: source
spec:
  name: k8s
  path: cloudquery/k8s
  version: "VERSION"
  tables: ["k8s_custom_resources"]
  destinations: ["postgresql"]
  spec:
    contexts: ["my-cluster"]
    custom_resources:
      - gvk: "cert-manager.io/v1/Certificate"
        namespaces: ["default", "production"]
      - gvk: "argoproj.io/v1alpha1/Application"
```

## Examples

### Query all certificates
```sql
SELECT 
  name, 
  namespace,
  spec->>'secretName' as secret_name,
  status->>'conditions' as conditions
FROM k8s_custom_resources
WHERE kind = 'Certificate' AND api_version = 'cert-manager.io/v1';
```
```

### 2. Configuration Documentation

**Location**: `docs/_configuration.md`

Add section on custom resources configuration with examples.

### 3. README Updates

**Location**: `README.md`

Add feature description and link to custom resources documentation.

## Execution Sequence

### Phase 1: Foundation (Priority P0)
1. ✅ Extend `Spec` struct with `CustomResources` field
2. ✅ Add `CustomResourceSpec` type with validation
3. ✅ Update `client/spec/schema.json` with new fields
4. ✅ Add validation in `Spec.Validate()`
5. ✅ Write unit tests for spec validation

**Deliverable**: Configuration schema with validation

### Phase 2: Client Integration (Priority P0)
1. ✅ Add `dynamicClients map[string]dynamic.Interface` to `Client`
2. ✅ Initialize dynamic clients in `Configure()`
3. ✅ Add `DynamicClient()` accessor method
4. ✅ Write unit tests for client initialization

**Deliverable**: Dynamic client available in plugin client

### Phase 3: Table Definition (Priority P1)
1. ✅ Create `resources/services/customresources/` package
2. ✅ Define `CustomResources()` table function
3. ✅ Define `CustomResourceRow` struct
4. ✅ Implement column definitions matching closed-source schema
5. ✅ Write helper functions for JSON resolvers
6. ✅ Unit tests for table structure

**Deliverable**: Table schema definition

### Phase 4: Fetcher Implementation (Priority P1)
1. ✅ Implement `customResourceMultiplex()`
2. ✅ Implement `fetchCustomResources()`
3. ✅ Implement `convertUnstructuredToRow()`
4. ✅ Add `parseGVK()` and `pluralize()` helpers
5. ✅ Handle pagination with Continue token
6. ✅ Error handling for missing CRDs/permissions
7. ✅ Write mock-based unit tests

**Deliverable**: Working CR fetcher with tests

### Phase 5: Integration & Testing (Priority P1)
1. ✅ Register table in `plugin.go`
2. ✅ Generate mocks for dynamic client
3. ✅ Integration tests with test cluster
4. ✅ Test with various CR types (cert-manager, ArgoCD, etc.)
5. ✅ Performance testing with large CR sets

**Deliverable**: Integrated and tested feature

### Phase 6: Documentation (Priority P2)
1. ✅ Create `docs/tables/k8s_custom_resources.md`
2. ✅ Update `docs/_configuration.md`
3. ✅ Update main `README.md`
4. ✅ Add configuration examples
5. ✅ Add SQL query examples

**Deliverable**: Complete documentation

### Phase 7: Advanced Features (Priority P3)
1. ⏸️ Wildcard pattern matching for GVK
2. ⏸️ Exclude patterns
3. ⏸️ CRD auto-discovery
4. ⏸️ Version preference logic

**Deliverable**: Enhanced CR selection features

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| Resource name pluralization incorrect | Medium | Fetch from CRD definition instead of hardcoded logic |
| Large CR objects cause OOM | High | Add size limits, pagination, streaming |
| Permission errors in multi-tenant clusters | Medium | Graceful error handling, continue on error |
| Schema incompatibility with closed-source | High | Thorough comparison testing with reference data |
| Performance impact on large clusters | Medium | Namespace filtering, selective GVK configuration |

## Success Criteria

- ✅ Users can specify CRs in configuration via GVK format
- ✅ CRs are fetched and stored in `k8s_custom_resources` table
- ✅ Schema matches CloudQuery closed-source plugin
- ✅ All tests pass (unit, integration)
- ✅ Documentation is complete and accurate
- ✅ No breaking changes to existing functionality
- ✅ Performance acceptable for clusters with 1000+ CRs

## Open Questions

1. **Pluralization Logic**: Should we fetch resource names from CRD definitions instead of hardcoding pluralization rules?
   - **Decision**: Start with simple pluralization, fetch from CRD if not found

2. **Wildcard Support**: Should Phase 1 include wildcard pattern matching?
   - **Decision**: No, defer to Phase 7 (P3)

3. **CRD Discovery**: Should we automatically discover all CRDs vs requiring explicit configuration?
   - **Decision**: Explicit configuration for Phase 1, auto-discovery in P3

4. **Version Selection**: When CR has multiple versions, which to prefer?
   - **Decision**: Use storage version from CRD definition

## References

- CloudQuery Plugin SDK: https://github.com/cloudquery/plugin-sdk
- Kubernetes client-go: https://github.com/kubernetes/client-go
- Dynamic Client: https://pkg.go.dev/k8s.io/client-go/dynamic
- Existing CRD table: `resources/services/crd/crds.go`
- Example resource table: `resources/services/core/nodes.go`
