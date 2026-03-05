# Feature Specification: CloudQuery Compatibility Updates

**Feature Branch**: `cq-custom-resource`  
**Created**: 2026-03-05  
**Status**: Draft  
**Parent Spec**: `001-cq-crd`

## Summary

Update the k8s_custom_resources table implementation to match CloudQuery's official k8s plugin schema and conventions. This ensures compatibility and follows CloudQuery best practices.

## Objectives

1. Match the table schema from https://hub.cloudquery.io/plugins/source/cloudquery/k8s/latest/tables/k8s_custom_resources
2. Use `uid` as primary key (with deterministic _cq_id)
3. Store JSON fields (labels, annotations, spec, status, owner_references) as proper JSON types (not strings)
4. Simplify table definition to follow CloudQuery patterns (like crds.go)
5. Update documentation to reflect correct primary key behavior

## Current Issues

1. **Column duplication**: Table definition specifies columns manually AND uses TransformWithStruct
2. **Wrong types**: Using `string` for JSON fields instead of `map[string]string` or `map[string]any`
3. **Missing primary key**: Not using `WithPrimaryKeys("UID")` for deterministic _cq_id
4. **Extra fields**: Has `gvk`, `resource_version`, `generation`, `created_at` not in CloudQuery schema
5. **Missing fields**: Lacks `owner_references` and `finalizers`
6. **JSON encoding**: Manually marshaling to JSON strings instead of letting SDK handle it
7. **Documentation**: Incorrect primary key information

## Requirements

### Functional Requirements

- **FR-001**: Table MUST use `uid` as primary key when write_mode is `overwrite` or `overwrite-delete-stale`
- **FR-002**: Table MUST have deterministic `_cq_id` generated from `uid`
- **FR-003**: JSON columns (labels, annotations, owner_references, spec, status) MUST be stored as jsonb in PostgreSQL
- **FR-004**: Table definition MUST follow CloudQuery patterns (minimal column specification)
- **FR-005**: Column names MUST match CloudQuery's schema: `api_version`, `kind` (not `gvk`)

### Schema Requirements

**Columns** (matching CloudQuery):
- `_cq_id` (UUID) - Deterministic hash of uid
- `_cq_parent_id` (UUID)
- `context` (String)
- `api_version` (String) - e.g., "cert-manager.io/v1"
- `kind` (String) - e.g., "Certificate"
- `namespace` (String)
- `name` (String)
- `uid` (String) - Primary key
- `labels` (JSON/map[string]string)
- `annotations` (JSON/map[string]string)
- `owner_references` (JSON/[]any)
- `finalizers` (List<String>/[]string)
- `spec` (JSON/map[string]any)
- `status` (JSON/map[string]any)

**PostgreSQL Table Schema** (when using PG destination):
```sql
     Column      |            Type             | Nullable
-----------------+-----------------------------+----------
 _cq_sync_time   | timestamp without time zone |
 _cq_source_name | text                        |
 _cq_id          | uuid                        | not null
 _cq_parent_id   | uuid                        |
 context         | text                        |
 kind            | text                        |
 api_version     | text                        |
 name            | text                        |
 namespace       | text                        |
 uid             | text                        | not null
 labels          | jsonb                       |
 annotations     | jsonb                       |
 owner_references| jsonb                       |
 finalizers      | text[]                      |
 spec            | jsonb                       |
 status          | jsonb                       |
```

**Primary Key/Constraints**:
- When `write_mode: overwrite` or `overwrite-delete-stale`: Primary key on `uid`, unique constraint on `_cq_id`
- When `write_mode: append`: No primary key

**Removed columns** (not in CloudQuery schema):
- `gvk` - Split into `api_version` and `kind`
- `resource_version` - Not needed in output
- `generation` - Not needed in output
- `created_at` - Not needed in output

## Implementation Plan

### Task 1: Simplify Table Definition

**File**: `resources/services/customresources/custom_resources.go`

**Changes**:
1. Remove manual column definitions
2. Add `transformers.WithPrimaryKeys("UID")`
3. Keep only `client.ContextColumn` in Columns list
4. Remove unused imports (json, time, arrow)

```go
func CustomResources() *schema.Table {
    return &schema.Table{
        Name:      "k8s_custom_resources",
        Resolver:  fetchCustomResources,
        Multiplex: client.ContextMultiplex,
        Transform: transformers.TransformWithStruct(&CustomResourceRow{}, transformers.WithPrimaryKeys("UID")),
        Columns:   schema.ColumnList{client.ContextColumn},
    }
}
```

### Task 2: Update CustomResourceRow Struct

**File**: `resources/services/customresources/custom_resources.go`

**Changes**:
1. Replace `GVK string` with `APIVersion string` and `Kind string`
2. Change `Labels` from `string` to `map[string]string`
3. Change `Annotations` from `string` to `map[string]string`
4. Add `OwnerReferences []any`
5. Add `Finalizers []string`
6. Change `Spec` from `string` to `map[string]any`
7. Change `Status` from `string` to `map[string]any`
8. Remove `ResourceVersion`, `Generation`, `CreatedAt`

```go
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
```

### Task 3: Update convertToCustomResourceRow Function

**File**: `resources/services/customresources/custom_resources.go`

**Changes**:
1. Remove JSON marshaling for labels/annotations
2. Directly assign map types
3. Extract owner references
4. Extract finalizers
5. Directly assign spec/status maps (no JSON encoding)

```go
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
```

### Task 4: Update Documentation

**File**: `docs/tables/k8s_custom_resources.md`

**Changes**:
1. Update primary key section to explain uid and deterministic _cq_id
2. Update columns table to match new schema
3. Remove old columns (gvk, resource_version, generation, created_at)
4. Add new columns (api_version, kind split from gvk, owner_references, finalizers)
5. Update types: String → JSON for appropriate fields

### Task 5: Update Tests

**Files**: `resources/services/customresources/*_test.go`

**Changes**:
1. Update test expectations for new struct fields
2. Remove tests for removed fields
3. Add tests for new fields (owner_references, finalizers)
4. Update assertions to check map types instead of JSON strings

## Success Criteria

- [ ] Build succeeds with no errors
- [ ] All tests pass
- [ ] Table schema matches CloudQuery's k8s_custom_resources
- [ ] uid is used as primary key (when write_mode is overwrite/overwrite-delete-stale)
- [ ] _cq_id is deterministic (hash of uid)
- [ ] JSON fields stored as jsonb in PostgreSQL
- [ ] finalizers stored as text[] in PostgreSQL
- [ ] Documentation accurately describes schema
- [ ] End-to-end test confirms data syncs correctly
- [ ] PostgreSQL table structure matches expected schema with correct column types

## Testing Strategy

1. **Unit tests**: Update existing tests for new struct
2. **Build verification**: `go build ./...`
3. **Test suite**: `go test ./resources/services/customresources/...`
4. **End-to-end**: Sync Widget CR and verify columns in PostgreSQL

## Notes

- The SDK's `transformers.WithPrimaryKeys("UID")` automatically creates a deterministic _cq_id
- Using map types instead of strings lets the SDK handle JSON encoding/decoding
- This follows the same pattern as other k8s tables (crds.go, pods, etc.)
- The `deterministic_cq_id` setting is handled by the cloudquery CLI, not the plugin
