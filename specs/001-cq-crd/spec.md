# Feature Specification: Custom Resources Support

**Feature Branch**: `cq-crd`  
**Created**: 2026-02-27  
**Status**: Draft  
**Input**: User description: "The kubernetes (k8s) CloudQuery plugin we maintain in the following repository has no support for storing custom resources like the closed source plugin does. Also, we would like to be able to specify which custom resources to include, not just include all of them like the closed source plugin. At least for phase 1, we will store the requested CRs in the same table as the closed source plugin with the same table columns: k8s_custom_resources."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Basic Custom Resource Collection (Priority: P1)

As a CloudQuery user, I want to specify which Kubernetes Custom Resources to collect in my source configuration so that I can sync specific CRs to my destination without collecting all CRs in the cluster.

**Why this priority**: This is the core MVP functionality. Without this, users cannot collect any custom resources at all. It provides immediate value by enabling selective CR collection, which is more efficient than the closed-source plugin's "all or nothing" approach.

**Independent Test**: Can be fully tested by configuring a single CR type (e.g., `apps.example.com/v1/MyApp`), running CloudQuery sync, and verifying that only that CR type appears in the `k8s_custom_resources` table.

**Acceptance Scenarios**:

1. **Given** a K8s cluster with multiple CRD types installed (e.g., CertManager certificates, ArgoCD applications)
   **When** I configure the plugin to collect only `cert-manager.io/v1/Certificate` resources
   **Then** only Certificate CRs are synced to the `k8s_custom_resources` table

2. **Given** a configuration with multiple CR types specified
   **When** I run CloudQuery sync
   **Then** all specified CR types are collected and stored with proper schema (apiVersion, kind, spec, status, metadata)

3. **Given** a CR configuration with namespace filtering
   **When** I sync with namespace constraints
   **Then** only CRs from specified namespaces are collected

---

### User Story 2 - Wildcard and Pattern Matching (Priority: P2)

As a CloudQuery user, I want to use wildcards and patterns to specify CR groups so that I can collect all CRs from a specific API group without listing each resource individually.

**Why this priority**: This enhances usability significantly for users with many related CRs (e.g., all cert-manager resources, all ArgoCD resources). It's not critical for MVP but greatly improves user experience.

**Independent Test**: Configure pattern like `cert-manager.io/*/` and verify all cert-manager CRs across all versions are collected.

**Acceptance Scenarios**:

1. **Given** a configuration with pattern `*.example.com/v1/`
   **When** CRDs exist for `apps.example.com/v1`, `configs.example.com/v1`
   **Then** all CRs from both groups are collected

2. **Given** a wildcard configuration `cert-manager.io/*/*`
   **When** certificates and issuers exist across v1 and v1alpha1 versions
   **Then** all cert-manager resources are collected regardless of version

---

### User Story 3 - Exclude Patterns for Fine-Grained Control (Priority: P3)

As a CloudQuery user, I want to exclude specific CRs or patterns from collection so that I can collect most CRs from a group while omitting specific types.

**Why this priority**: This is a convenience feature for advanced users. Most use cases can be handled with explicit inclusion lists from P1/P2.

**Independent Test**: Configure inclusion pattern `*.example.com/v1/*` with exclusion `secrets.example.com/v1/Secret`, verify all CRs collected except the excluded type.

**Acceptance Scenarios**:

1. **Given** an include pattern `*.cert-manager.io/*/*` and exclude pattern `challenges.acme.cert-manager.io/*/*`
   **When** sync runs
   **Then** all cert-manager CRs are collected except ACME challenges

---

### Edge Cases

- **Empty CRD list**: When no CRDs match the specified patterns, sync completes successfully with no CR data collected
- **Invalid CRD references**: When a specified CR type doesn't exist, log warning and continue with other CRs
- **CRD version deprecation**: When a CRD has multiple versions and one is deprecated, prefer the storage version
- **Large CR objects**: When CR spec/status exceeds reasonable size (>1MB), handle gracefully without OOM
- **Cluster without CRDs**: When target cluster has no CRDs installed, sync completes successfully with informational log
- **Permission errors**: When service account lacks permission to list specific CRs, log error for that CR type and continue
- **CR without spec or status**: When CR has only metadata (no spec/status), store with null values for those fields
- **Concurrent CRD changes**: When CRDs are created/deleted during sync, handle gracefully without crashing

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow users to specify custom resources to collect via source configuration
- **FR-002**: System MUST store collected CRs in table named `k8s_custom_resources` with schema matching CloudQuery's closed-source plugin
- **FR-003**: System MUST support CR specification in format `group/version/kind` (e.g., `cert-manager.io/v1/Certificate`)
- **FR-004**: System MUST extract and store CR metadata (name, namespace, uid, labels, annotations, owner_references, finalizers)
- **FR-005**: System MUST store CR spec and status as JSON columns
- **FR-006**: System MUST include context column to identify which K8s context the CR came from
- **FR-007**: System SHOULD use SDK-generated `_cq_id` as the primary key for the `k8s_custom_resources` table (updated: removed explicit primary keys to let SDK auto-generate)
- **FR-008**: System MUST support wildcard patterns for group, version, and kind (e.g., `*.cert-manager.io/*/*`) [DEFERRED to Phase 7]
- **FR-009**: System MUST validate CR specifications at plugin initialization and fail fast with clear error messages
- **FR-010**: System MUST use dynamic client to fetch CRs based on GVK (GroupVersionKind)
- **FR-011**: System MUST handle pagination when listing CRs to avoid memory issues with large result sets
- **FR-012**: System MUST respect the plugin's concurrency settings when fetching CRs
- **FR-013**: System MUST support empty CR list (collect no CRs) as valid configuration
- **FR-014**: Configuration format MUST be YAML-based and follow existing plugin patterns [NEEDS IMPLEMENTATION DECISION: exact YAML structure]

### Key Entities

- **CustomResourceSpec**: Configuration entity defining which CRs to collect
  - group: API group (e.g., "cert-manager.io")
  - version: API version (e.g., "v1")
  - kind: Resource kind (e.g., "Certificate")
  - namespaces: Optional namespace filter (list or wildcard)
  
- **CustomResource**: Runtime representation of collected CR
  - context: K8s context name
  - apiVersion: Combined group/version
  - kind: Resource kind
  - metadata: Standard K8s metadata (name, namespace, uid, labels, annotations, etc.)
  - spec: CR specification as JSON
  - status: CR status as JSON
  - _cq_id: Primary key (auto-generated by SDK)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can configure and successfully sync at least one CR type to the `k8s_custom_resources` table
- **SC-002**: Table schema contains all required columns (context, uid, api_version, kind, name, namespace, labels, annotations, owner_references, finalizers, spec, status) plus CloudQuery SDK columns (_cq_id, _cq_parent_id auto-generated)
- **SC-003**: Plugin handles clusters with 100+ CRD types and 10,000+ CR instances without crashing
- **SC-004**: Configuration validation provides clear error messages for invalid GVK specifications (e.g., "Invalid GVK format: expected 'group/version/kind', got 'invalid'")
- **SC-005**: Wildcard patterns work correctly across different test cases (group wildcard, version wildcard, kind wildcard, combinations)
- **SC-006**: Plugin can sync CRs from multiple contexts simultaneously when multiple contexts are configured

## Implementation Approach

### Chosen Approach: Compact GVK String Format

For Phase 1, we use a compact string format following Kubernetes GVK notation:

```yaml
kind: source
spec:
  name: k8s
  path: ./k8s
  version: v1.0.0
  destinations: [destination]
  spec:
    contexts: ["*"]
    custom_resources:
      - gvk: "cert-manager.io/v1/Certificate"
      - gvk: "cert-manager.io/v1/Issuer"
        namespaces: ["default", "production"]  # Optional namespace filter
      - gvk: "argoproj.io/v1alpha1/Application"
```

**Rationale**:
- **Compact**: Single string instead of three fields
- **Standard**: Matches Kubernetes GVK notation
- **Simple**: Easy to parse and validate with single regex
- **Extensible**: Can add wildcards in future (e.g., `cert-manager.io/*/Certificate`)
- **Clear**: Format is self-documenting

**Format**: `"<group>/<version>/<kind>"`
- `group`: API group (e.g., `cert-manager.io`, `argoproj.io`)
- `version`: API version (e.g., `v1`, `v1alpha1`)
- `kind`: Resource kind with capital first letter (e.g., `Certificate`, `Application`)

### Future Enhancements (Phase 2+)

Wildcard pattern support can be added later:

```yaml
custom_resources:
  - gvk: "cert-manager.io/v1/*"        # All v1 cert-manager resources
  - gvk: "cert-manager.io/*/Certificate" # Certificates across versions
  - gvk: "*.example.com/*/*"            # All resources from example.com
```

Exclude patterns:

```yaml
custom_resources:
  - gvk: "cert-manager.io/v1/*"
    exclude:
      - "cert-manager.io/v1/Challenge"  # Exclude specific resources
```

**Note**: Phase 1 implementation does not support wildcards. Each GVK must be explicitly specified.

## Technical Design Considerations

### 1. Configuration Schema Extension

Extend `client/spec/spec.go`:

```go
type Spec struct {
    Contexts        []string               `yaml:"contexts,omitempty" json:"contexts"`
    Concurrency     int                    `yaml:"concurrency,omitempty" json:"concurrency"`
    CustomResources []CustomResourceSpec   `yaml:"custom_resources,omitempty" json:"custom_resources"`
}

type CustomResourceSpec struct {
    // GVK in format "group/version/kind" (e.g., "cert-manager.io/v1/Certificate")
    GVK        string   `yaml:"gvk" json:"gvk" jsonschema:"required,pattern=^[a-z0-9.-]+/[a-z0-9]+/[A-Z][a-zA-Z0-9]*$"`
    
    // Optional: Namespace filter (empty = all namespaces)
    Namespaces []string `yaml:"namespaces,omitempty" json:"namespaces"`
}
```

**Validation Pattern**: `^[a-z0-9.-]+/[a-z0-9]+/[A-Z][a-zA-Z0-9]*$`
- Group: lowercase alphanumeric with dots and dashes
- Version: lowercase alphanumeric
- Kind: Must start with uppercase letter

### 2. Table Schema Definition

Create `resources/services/customresource/custom_resources.go`:

```go
func CustomResources() *schema.Table {
    return &schema.Table{
        Name:      "k8s_custom_resources",
        Resolver:  fetchCustomResources,
        Multiplex: client.ContextMultiplex,
        Transform: client.TransformWithStruct(new(CustomResource)),
        Columns: schema.ColumnList{
            client.ContextColumn,
            schema.Column{Name: "uid", Type: arrow.BinaryTypes.String, PrimaryKey: true},
            schema.Column{Name: "api_version", Type: arrow.BinaryTypes.String},
            schema.Column{Name: "kind", Type: arrow.BinaryTypes.String},
            schema.Column{Name: "name", Type: arrow.BinaryTypes.String},
            schema.Column{Name: "namespace", Type: arrow.BinaryTypes.String},
            schema.Column{Name: "labels", Type: types.ExtensionTypes.JSON},
            schema.Column{Name: "annotations", Type: types.ExtensionTypes.JSON},
            schema.Column{Name: "owner_references", Type: types.ExtensionTypes.JSON},
            schema.Column{Name: "finalizers", Type: arrow.ListOf(arrow.BinaryTypes.String)},
            schema.Column{Name: "spec", Type: types.ExtensionTypes.JSON},
            schema.Column{Name: "status", Type: types.ExtensionTypes.JSON},
        },
    }
}
```

### 3. Dynamic Client Usage

Use K8s dynamic client to fetch arbitrary CRs:

```go
import (
    "k8s.io/client-go/dynamic"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

func fetchCustomResources(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource, res chan<- any) error {
    cl := meta.(*client.Client)
    dynamicClient := cl.DynamicClient() // Need to add this to client
    
    spec := cl.Spec.CustomResources
    if spec == nil || len(spec.Resources) == 0 {
        return nil // No CRs configured
    }
    
    for _, crSpec := range spec.Resources {
        gvr := schema.GroupVersionResource{
            Group:    crSpec.Group,
            Version:  crSpec.Version,
            Resource: pluralize(crSpec.Kind), // Need helper to pluralize
        }
        
        // Fetch using dynamic client
        list, err := dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
        if err != nil {
            // Log warning and continue with other CRs
            continue
        }
        
        for _, item := range list.Items {
            cr := convertToCustomResource(item)
            res <- cr
        }
    }
    
    return nil
}
```

### 4. Testing Strategy

- **Unit tests**: Test CR spec parsing, validation, pattern matching (if implemented)
- **Integration tests**: 
  - Create test CRDs in test cluster
  - Create CR instances
  - Verify collection and schema
  - Test with mock K8s client
- **E2E tests**: Test against real cluster with common CRDs (cert-manager, ArgoCD if available)

### 5. Documentation Updates

- Update `docs/configuration.md` with CR configuration examples
- Create `docs/tables/k8s_custom_resources.md` with table schema
- Add examples for common CRDs (cert-manager, ArgoCD, etc.)
- Document limitations and edge cases

## Phase 1 Deliverables

1. **Configuration schema** with explicit list support (Option A)
2. **Table definition** matching closed-source plugin schema
3. **Dynamic client integration** for fetching arbitrary CRs
4. **Basic resolver** with pagination support
5. **Unit tests** for configuration parsing and validation
6. **Integration tests** with mock CRDs
7. **Documentation** for configuration and table schema
8. **Example configurations** for common use cases

## Future Enhancements (Post-Phase 1)

- **Pattern/wildcard support** (Option B features)
- **Namespace filtering** at CR level
- **Label selectors** for fine-grained filtering
- **Exclude patterns** for inverse filtering
- **Performance optimization** with concurrent CR fetching per type
- **CRD discovery mode** to list available CRDs for configuration
- **Relationship tracking** between CRs and their parent CRDs

## Open Questions

1. **Pluralization**: How to reliably convert Kind to Resource name? (e.g., "Certificate" → "certificates", "Ingress" → "ingresses")
   - Could parse from CRD definition
   - Could require users to specify resource name
   - Could use inflection library

2. **Large CRs**: Should we set a size limit on spec/status fields?
   - Option: Truncate with warning
   - Option: Store reference to object instead
   - Option: Make configurable

3. **CRD changes during sync**: How to handle CRDs being added/removed while sync is running?
   - Option: Snapshot CRD list at start
   - Option: Fail-safe continue on errors
   - Option: Retry with updated CRD list

4. **Default behavior**: If `custom_resources` config is not specified, should we:
   - Collect nothing (recommended for phase 1 - backward compatible)
   - Collect all CRs (matches closed-source plugin)
   - Make it an error (force explicit configuration)

## Recommendation

**For Phase 1**: Implement Option A (list-based configuration) with the following:
- Explicit GVK specification (no wildcards yet)
- Collect nothing by default (backward compatible)
- Clear validation and error messages
- Table schema matching closed-source plugin exactly
- Comprehensive tests and documentation

This provides immediate value, is simple to implement and test, and lays groundwork for pattern-based enhancements in future phases.

---

## DETAILED CONFIGURATION FORMAT DECISION

### Final Decision: Option A (List-Based) for Phase 1

After analyzing the existing codebase patterns, **Option A is the clear choice** for the following reasons:

1. **Consistency with existing code**: All other K8s resources use explicit typed clients (e.g., `AppsV1().Deployments()`). Custom resources need a different approach, but explicit configuration aligns with the overall plugin philosophy.

2. **Easy validation**: The spec validation can happen at plugin initialization, providing immediate feedback to users.

3. **Clear user intent**: No ambiguity about which resources are collected - users explicitly state what they want.

4. **Testability**: Easy to write deterministic tests with known inputs/outputs.

5. **Foundation for future enhancements**: Wildcard support can be added later without breaking existing configurations.

### Configuration Schema (Final)

```yaml
kind: source
spec:
  name: k8s
  path: ./k8s
  version: v1.0.0
  destinations: [postgresql]
  spec:
    contexts: ["*"]  # Existing field
    concurrency: 50000  # Existing field
    custom_resources:  # NEW FIELD
      - group: cert-manager.io
        version: v1
        kind: Certificate
        namespaces: []  # Optional: empty = all namespaces
      - group: cert-manager.io
        version: v1
        kind: Issuer
      - group: cert-manager.io
        version: v1
        kind: ClusterIssuer
      - group: argoproj.io
        version: v1alpha1
        kind: Application
        namespaces: ["argocd"]  # Only from argocd namespace
```

### Alternative Simpler Format (Also Valid)

For users who prefer simplicity, also support string format:

```yaml
kind: source
spec:
  name: k8s
  path: ./k8s
  version: v1.0.0
  spec:
    custom_resources:
      - "cert-manager.io/v1/Certificate"
      - "cert-manager.io/v1/Issuer"
      - "argoproj.io/v1alpha1/Application"
```

The code will parse both formats and convert string format to structured format internally.

---

## DETAILED IMPLEMENTATION GUIDE

### Phase 1.1: Configuration Schema (Week 1)

#### Step 1: Update Spec Struct

**File**: `client/spec/spec.go`

```go
package spec

import _ "embed"

type Spec struct {
    Contexts    []string              `yaml:"contexts,omitempty" json:"contexts" jsonschema:"minLength=1"`
    Concurrency int                   `yaml:"concurrency,omitempty" json:"concurrency" jsonschema:"minimum=1,default=50000"`
    
    // NEW: Custom resources to collect
    CustomResources []CustomResourceSpec `yaml:"custom_resources,omitempty" json:"custom_resources,omitempty"`
}

// CustomResourceSpec defines a single custom resource type to collect
type CustomResourceSpec struct {
    // API Group (e.g., "cert-manager.io", "argoproj.io")
    Group   string `yaml:"group" json:"group" jsonschema:"required"`
    
    // API Version (e.g., "v1", "v1alpha1")
    Version string `yaml:"version" json:"version" jsonschema:"required"`
    
    // Resource Kind (e.g., "Certificate", "Application")
    Kind    string `yaml:"kind" json:"kind" jsonschema:"required"`
    
    // Optional: namespace filter. Empty means all namespaces.
    Namespaces []string `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`
}

// UnmarshalYAML supports both structured and string formats
func (crs *CustomResourceSpec) UnmarshalYAML(unmarshal func(interface{}) error) error {
    // Try string format first (e.g., "cert-manager.io/v1/Certificate")
    var str string
    if err := unmarshal(&str); err == nil {
        return crs.parseFromString(str)
    }
    
    // Fall back to structured format
    type Alias CustomResourceSpec
    var alias Alias
    if err := unmarshal(&alias); err != nil {
        return err
    }
    *crs = CustomResourceSpec(alias)
    return crs.Validate()
}

func (crs *CustomResourceSpec) parseFromString(s string) error {
    parts := strings.Split(s, "/")
    if len(parts) != 3 {
        return fmt.Errorf("invalid custom resource format %q: expected 'group/version/kind'", s)
    }
    crs.Group = parts[0]
    crs.Version = parts[1]
    crs.Kind = parts[2]
    return crs.Validate()
}

func (crs *CustomResourceSpec) Validate() error {
    if crs.Group == "" {
        return fmt.Errorf("custom resource group cannot be empty")
    }
    if crs.Version == "" {
        return fmt.Errorf("custom resource version cannot be empty")
    }
    if crs.Kind == "" {
        return fmt.Errorf("custom resource kind cannot be empty")
    }
    // Validate group format (must be valid DNS subdomain)
    if !isValidAPIGroup(crs.Group) {
        return fmt.Errorf("invalid API group %q: must be a valid DNS subdomain", crs.Group)
    }
    return nil
}

func isValidAPIGroup(group string) bool {
    // Simple validation: lowercase alphanumeric + dots + hyphens
    // More comprehensive validation can be added
    matched, _ := regexp.MatchString(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`, group)
    return matched
}

// GVK returns the GroupVersionKind representation
func (crs *CustomResourceSpec) GVK() schema.GroupVersionKind {
    return schema.GroupVersionKind{
        Group:   crs.Group,
        Version: crs.Version,
        Kind:    crs.Kind,
    }
}

// GVR returns the GroupVersionResource representation
// This uses a pluralization heuristic
func (crs *CustomResourceSpec) GVR() schema.GroupVersionResource {
    return schema.GroupVersionResource{
        Group:    crs.Group,
        Version:  crs.Version,
        Resource: pluralize(crs.Kind),
    }
}

// pluralize converts Kind to Resource name (simple heuristic)
func pluralize(kind string) string {
    lower := strings.ToLower(kind)
    
    // Special cases
    specialCases := map[string]string{
        "endpoints":        "endpoints",
        "ingress":         "ingresses",
        "networkpolicy":   "networkpolicies",
        "podsecuritypolicy": "podsecuritypolicies",
    }
    
    if plural, ok := specialCases[strings.ReplaceAll(lower, " ", "")]; ok {
        return plural
    }
    
    // General rules
    if strings.HasSuffix(lower, "s") || strings.HasSuffix(lower, "x") || 
       strings.HasSuffix(lower, "ch") || strings.HasSuffix(lower, "sh") {
        return lower + "es"
    }
    if strings.HasSuffix(lower, "y") && len(lower) > 1 {
        // Check if preceded by consonant
        prevChar := lower[len(lower)-2]
        if !strings.ContainsRune("aeiou", rune(prevChar)) {
            return lower[:len(lower)-1] + "ies"
        }
    }
    
    return lower + "s"
}

func (s *Spec) SetDefaults() {
    if s.Concurrency <= 0 {
        const defaultConcurrency = 50000
        s.Concurrency = defaultConcurrency
    }
}

//go:embed schema.json
var JSONSchema string
```

#### Step 2: Update JSON Schema

**File**: `client/spec/schema.json`

Add to the `Spec` properties:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/cloudquery/cloudquery/plugins/source/k8s/client/spec/spec",
  "$ref": "#/$defs/Spec",
  "$defs": {
    "Spec": {
      "properties": {
        "contexts": { /* existing */ },
        "concurrency": { /* existing */ },
        "custom_resources": {
          "oneOf": [
            {
              "items": {
                "$ref": "#/$defs/CustomResourceSpec"
              },
              "type": "array",
              "description": "List of custom resources to collect. Each entry specifies a GVK (Group/Version/Kind) to sync."
            },
            {
              "type": "null"
            }
          ]
        }
      },
      "additionalProperties": false,
      "type": "object"
    },
    "CustomResourceSpec": {
      "properties": {
        "group": {
          "type": "string",
          "description": "API Group (e.g., 'cert-manager.io')",
          "minLength": 1
        },
        "version": {
          "type": "string",
          "description": "API Version (e.g., 'v1', 'v1alpha1')",
          "minLength": 1
        },
        "kind": {
          "type": "string",
          "description": "Resource Kind (e.g., 'Certificate', 'Application')",
          "minLength": 1
        },
        "namespaces": {
          "type": "array",
          "items": {
            "type": "string"
          },
          "description": "Optional namespace filter. Empty means all namespaces."
        }
      },
      "required": ["group", "version", "kind"],
      "type": "object"
    }
  }
}
```

#### Step 3: Add Dynamic Client to Client

**File**: `client/client.go`

Add dynamic client support:

```go
import (
    "k8s.io/client-go/dynamic"
)

type Client struct {
    logger zerolog.Logger
    clients map[string]kubernetes.Interface
    namespaces map[string][]v1.Namespace
    apiExtensions map[string]apiextensionsclientset.Interface
    
    // NEW: Add dynamic client
    dynamicClients map[string]dynamic.Interface
    
    spec     *spec.Spec
    contexts []string
    paths    map[string]struct{}
    Context   string
    Namespace string
}

func (c *Client) DynamicClient() dynamic.Interface {
    return c.dynamicClients[c.Context]
}

func Configure(ctx context.Context, logger zerolog.Logger, s spec.Spec) (schema.ClientMeta, error) {
    // ... existing code ...
    
    c := Client{
        logger:        logger,
        clients:       make(map[string]kubernetes.Interface),
        namespaces:    make(map[string][]v1.Namespace),
        apiExtensions: make(map[string]apiextensionsclientset.Interface),
        dynamicClients: make(map[string]dynamic.Interface), // NEW
        spec:          &s,
        contexts:      contexts,
        Context:       contexts[0],
        paths:         make(map[string]struct{}),
    }
    
    for _, ctxName := range contexts {
        logger.Info().Str("context", ctxName).Msg("creating k8s client for context")
        restConfig, err := buildRESTConfig(logger, rawKubeConfig, ctxName)
        if err != nil {
            return nil, fmt.Errorf("failed to build k8s REST config for context %q: %w", ctxName, err)
        }
        
        // ... existing client creation ...
        
        // NEW: Create dynamic client
        dynClient, err := dynamic.NewForConfig(restConfig)
        if err != nil {
            return nil, fmt.Errorf("failed to build k8s dynamic client for context %q: %w", ctxName, err)
        }
        c.dynamicClients[ctxName] = dynClient
        
        // ... rest of existing code ...
    }
    
    // NEW: Validate custom resources configuration
    if err := validateCustomResources(ctx, &c, logger); err != nil {
        return nil, fmt.Errorf("custom resources validation failed: %w", err)
    }
    
    return &c, nil
}

// validateCustomResources checks that specified CRDs exist
func validateCustomResources(ctx context.Context, c *Client, logger zerolog.Logger) error {
    if len(c.spec.CustomResources) == 0 {
        return nil // No CRs to validate
    }
    
    logger.Info().Msgf("Validating %d custom resource specifications", len(c.spec.CustomResources))
    
    // Validate each CR spec
    for i, crSpec := range c.spec.CustomResources {
        if err := crSpec.Validate(); err != nil {
            return fmt.Errorf("custom resource spec %d is invalid: %w", i, err)
        }
        
        logger.Debug().
            Str("group", crSpec.Group).
            Str("version", crSpec.Version).
            Str("kind", crSpec.Kind).
            Msg("Custom resource spec validated")
    }
    
    return nil
}
```

### Phase 1.2: Custom Resources Table (Week 2)

#### Step 4: Create Custom Resource Type

**File**: `resources/services/customresource/types.go` (NEW FILE)

```go
package customresource

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// CustomResource represents a K8s custom resource for CloudQuery
type CustomResource struct {
    // Context is the K8s context name
    Context string `json:"context"`
    
    // APIVersion is the group/version (e.g., "cert-manager.io/v1")
    APIVersion string `json:"api_version"`
    
    // Kind is the resource kind (e.g., "Certificate")
    Kind string `json:"kind"`
    
    // Name is the resource name
    Name string `json:"name"`
    
    // Namespace is the resource namespace (empty for cluster-scoped)
    Namespace string `json:"namespace"`
    
    // UID is the unique identifier
    UID string `json:"uid"`
    
    // Labels are resource labels
    Labels map[string]string `json:"labels"`
    
    // Annotations are resource annotations
    Annotations map[string]string `json:"annotations"`
    
    // OwnerReferences are the owner references
    OwnerReferences []metav1.OwnerReference `json:"owner_references"`
    
    // Finalizers are resource finalizers
    Finalizers []string `json:"finalizers"`
    
    // Spec is the resource spec as JSON
    Spec map[string]interface{} `json:"spec"`
    
    // Status is the resource status as JSON
    Status map[string]interface{} `json:"status"`
}

// FromUnstructured converts an unstructured K8s object to CustomResource
func FromUnstructured(obj *unstructured.Unstructured, context string) *CustomResource {
    cr := &CustomResource{
        Context:         context,
        APIVersion:      obj.GetAPIVersion(),
        Kind:            obj.GetKind(),
        Name:            obj.GetName(),
        Namespace:       obj.GetNamespace(),
        UID:             string(obj.GetUID()),
        Labels:          obj.GetLabels(),
        Annotations:     obj.GetAnnotations(),
        OwnerReferences: obj.GetOwnerReferences(),
        Finalizers:      obj.GetFinalizers(),
    }
    
    // Extract spec
    if spec, found, err := unstructured.NestedMap(obj.Object, "spec"); found && err == nil {
        cr.Spec = spec
    }
    
    // Extract status
    if status, found, err := unstructured.NestedMap(obj.Object, "status"); found && err == nil {
        cr.Status = status
    }
    
    return cr
}
```

#### Step 5: Create Table Definition

**File**: `resources/services/customresource/custom_resources.go` (NEW FILE)

```go
package customresource

import (
    "context"
    "fmt"
    
    "github.com/apache/arrow/go/v16/arrow"
    "github.com/cloudquery/cloudquery/plugins/source/k8s/client"
    "github.com/cloudquery/plugin-sdk/v4/schema"
    "github.com/cloudquery/plugin-sdk/v4/types"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

func CustomResources() *schema.Table {
    return &schema.Table{
        Name:      "k8s_custom_resources",
        Resolver:  fetchCustomResources,
        Multiplex: client.ContextMultiplex,
        Columns: schema.ColumnList{
            {
                Name:       "context",
                Type:       arrow.BinaryTypes.String,
                Resolver:   client.ResolveContext,
                PrimaryKey: false,
            },
            {
                Name:       "uid",
                Type:       arrow.BinaryTypes.String,
                PrimaryKey: true,
            },
            {
                Name: "api_version",
                Type: arrow.BinaryTypes.String,
            },
            {
                Name: "kind",
                Type: arrow.BinaryTypes.String,
            },
            {
                Name: "name",
                Type: arrow.BinaryTypes.String,
            },
            {
                Name: "namespace",
                Type: arrow.BinaryTypes.String,
            },
            {
                Name: "labels",
                Type: types.ExtensionTypes.JSON,
            },
            {
                Name: "annotations",
                Type: types.ExtensionTypes.JSON,
            },
            {
                Name: "owner_references",
                Type: types.ExtensionTypes.JSON,
            },
            {
                Name: "finalizers",
                Type: arrow.ListOf(arrow.BinaryTypes.String),
            },
            {
                Name: "spec",
                Type: types.ExtensionTypes.JSON,
            },
            {
                Name: "status",
                Type: types.ExtensionTypes.JSON,
            },
        },
    }
}

func fetchCustomResources(ctx context.Context, meta schema.ClientMeta, parent *schema.Resource, res chan<- any) error {
    cl := meta.(*client.Client)
    
    // Check if custom resources are configured
    if cl.Spec().CustomResources == nil || len(cl.Spec().CustomResources) == 0 {
        cl.Logger().Debug().Msg("No custom resources configured, skipping")
        return nil
    }
    
    dynClient := cl.DynamicClient()
    currentContext := cl.Context
    
    // Fetch each configured custom resource type
    for _, crSpec := range cl.Spec().CustomResources {
        gvr := schema.GroupVersionResource{
            Group:    crSpec.Group,
            Version:  crSpec.Version,
            Resource: crSpec.GVR().Resource,
        }
        
        cl.Logger().Debug().
            Str("group", crSpec.Group).
            Str("version", crSpec.Version).
            Str("kind", crSpec.Kind).
            Str("resource", gvr.Resource).
            Msg("Fetching custom resources")
        
        // Determine namespace scope
        namespaces := crSpec.Namespaces
        if len(namespaces) == 0 {
            // Try cluster-wide first, fall back to all namespaces
            if err := fetchCRsClusterWide(ctx, dynClient, gvr, currentContext, res, cl); err != nil {
                // If cluster-wide fails, try namespace-scoped across all namespaces
                cl.Logger().Debug().Err(err).Msg("Cluster-wide fetch failed, trying namespace-scoped")
                if err := fetchCRsAllNamespaces(ctx, dynClient, gvr, currentContext, res, cl); err != nil {
                    cl.Logger().Warn().
                        Err(err).
                        Str("gvr", fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)).
                        Msg("Failed to fetch custom resource, skipping")
                    continue
                }
            }
        } else {
            // Fetch from specific namespaces
            for _, ns := range namespaces {
                if err := fetchCRsInNamespace(ctx, dynClient, gvr, ns, currentContext, res, cl); err != nil {
                    cl.Logger().Warn().
                        Err(err).
                        Str("namespace", ns).
                        Str("gvr", fmt.Sprintf("%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)).
                        Msg("Failed to fetch custom resource from namespace, skipping")
                }
            }
        }
    }
    
    return nil
}

func fetchCRsClusterWide(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource, 
                         context string, res chan<- any, cl *client.Client) error {
    opts := metav1.ListOptions{}
    
    for {
        result, err := dynClient.Resource(gvr).List(ctx, opts)
        if err != nil {
            return err
        }
        
        for _, item := range result.Items {
            cr := FromUnstructured(&item, context)
            res <- cr
        }
        
        if result.GetContinue() == "" {
            break
        }
        opts.Continue = result.GetContinue()
    }
    
    return nil
}

func fetchCRsAllNamespaces(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource,
                           context string, res chan<- any, cl *client.Client) error {
    opts := metav1.ListOptions{}
    
    for {
        result, err := dynClient.Resource(gvr).Namespace("").List(ctx, opts)
        if err != nil {
            return err
        }
        
        for _, item := range result.Items {
            cr := FromUnstructured(&item, context)
            res <- cr
        }
        
        if result.GetContinue() == "" {
            break
        }
        opts.Continue = result.GetContinue()
    }
    
    return nil
}

func fetchCRsInNamespace(ctx context.Context, dynClient dynamic.Interface, gvr schema.GroupVersionResource,
                         namespace string, context string, res chan<- any, cl *client.Client) error {
    opts := metav1.ListOptions{}
    
    for {
        result, err := dynClient.Resource(gvr).Namespace(namespace).List(ctx, opts)
        if err != nil {
            return err
        }
        
        for _, item := range result.Items {
            cr := FromUnstructured(&item, context)
            res <- cr
        }
        
        if result.GetContinue() == "" {
            break
        }
        opts.Continue = result.GetContinue()
    }
    
    return nil
}
```

#### Step 6: Add Spec Accessor

**File**: `client/client.go`

Add method to access spec:

```go
func (c *Client) Spec() *spec.Spec {
    return c.spec
}
```

#### Step 7: Register Table in Plugin

**File**: `resources/plugin/plugin.go`

Add import and registration:

```go
import (
    // ... existing imports ...
    "github.com/cloudquery/cloudquery/plugins/source/k8s/resources/services/customresource"
)

func Plugin() *plugin.Plugin {
    return plugin.NewPlugin(
        "k8s",
        version.Version(),
        []*schema.Table{
            // ... existing tables ...
            crd.CRDs(),
            customresource.CustomResources(), // NEW
            // ... rest of tables ...
        },
        client.Configure,
    )
}
```

### Phase 1.3: Testing (Week 3)

Create comprehensive tests following existing patterns. Tests should cover:
- Configuration parsing
- GVR conversion
- CR fetching
- Error handling
- Namespace filtering

### Summary of Implementation Decisions

1. **Configuration**: Option A (list-based) with support for both structured and string formats
2. **Dynamic Client**: Added to Client struct, initialized per-context
3. **Pluralization**: Simple heuristic with special cases (can be improved by reading from CRD)
4. **Validation**: At plugin initialization, fail-fast with clear errors
5. **Namespace Handling**: Try cluster-wide first, fall back to namespace-scoped
6. **Error Handling**: Log warnings and continue with other CRs on errors
7. **Table Schema**: Exact match to closed-source plugin using Arrow types
