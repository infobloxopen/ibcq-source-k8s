# Implementation Plan: CloudQuery Compatibility Updates

**Spec**: `002-cloudquery-compat`  
**Created**: 2026-03-05  
**Estimated Time**: 2-3 hours

## Overview

Update k8s_custom_resources to match CloudQuery's official schema and best practices.

## Phase 1: Code Updates (1.5 hours)

### Task 1.1: Simplify Table Definition
**Time**: 20 minutes  
**File**: `resources/services/customresources/custom_resources.go`

**Steps**:
1. Remove all manual column definitions (keep only ContextColumn)
2. Add `transformers.WithPrimaryKeys("UID")` to Transform
3. Remove unused imports: `encoding/json`, `time`, `github.com/apache/arrow/go/v16/arrow`

**Acceptance**: Table definition matches crds.go pattern (< 10 lines)

---

### Task 1.2: Update CustomResourceRow Struct
**Time**: 15 minutes  
**File**: `resources/services/customresources/custom_resources.go`

**Steps**:
1. Replace `GVK string` → `APIVersion string` and `Kind string`
2. Change `Labels string` → `Labels map[string]string`
3. Change `Annotations string` → `Annotations map[string]string`
4. Add `OwnerReferences []any`
5. Add `Finalizers []string`
6. Change `Spec string` → `Spec map[string]any`
7. Change `Status string` → `Status map[string]any`
8. Remove: `ResourceVersion`, `Generation`, `CreatedAt`

**Acceptance**: Struct has 12 fields matching CloudQuery schema

---

### Task 1.3: Update convertToCustomResourceRow
**Time**: 30 minutes  
**File**: `resources/services/customresources/custom_resources.go`

**Steps**:
1. Set `APIVersion` and `Kind` from object (not `gvk` parameter)
2. Directly assign `Labels` map (no JSON marshaling)
3. Directly assign `Annotations` map (no JSON marshaling)
4. Extract and assign `OwnerReferences`
5. Extract and assign `Finalizers`
6. Directly assign `Spec` map (no JSON marshaling)
7. Directly assign `Status` map (no JSON marshaling)
8. Remove all `json.Marshal()` calls
9. Remove timestamp formatting

**Acceptance**: No JSON marshaling, direct map assignments

---

### Task 1.4: Update Documentation
**Time**: 15 minutes  
**File**: `docs/tables/k8s_custom_resources.md`

**Steps**:
1. Update "Primary Key" section to explain uid and deterministic _cq_id
2. Update columns table:
   - Remove: gvk, resource_version, generation, created_at
   - Add: api_version, kind, owner_references, finalizers
   - Update types: String → JSON for maps
3. Add note about jsonb storage in PostgreSQL
4. Add note about deterministic _cq_id

**Acceptance**: Documentation matches actual schema

---

## Phase 2: Testing (1 hour)

### Task 2.1: Update Unit Tests
**Time**: 30 minutes  
**Files**: `resources/services/customresources/custom_resources_test.go`

**Steps**:
1. Update `TestCustomResourceRow_Structure` for new fields
2. Remove tests for removed fields (GVK, ResourceVersion, Generation, CreatedAt)
3. Update `TestConvertToCustomResourceRow` expectations
4. Add checks for map types (not strings)
5. Update `TestCustomResources_ColumnTypes` test

**Acceptance**: All unit tests pass

---

### Task 2.2: Update Integration Tests
**Time**: 20 minutes  
**Files**: `resources/services/customresources/integration_test.go`

**Steps**:
1. Update test object creation for new struct
2. Update assertions to check maps not JSON strings
3. Add tests for owner_references and finalizers

**Acceptance**: All integration tests pass

---

### Task 2.3: Build and End-to-End Test
**Time**: 10 minutes

**Steps**:
1. Run `go build -o k8s-source .`
2. Sync Widget CR to PostgreSQL
3. Query `k8s_custom_resources` table
4. Verify PostgreSQL schema matches expected:
   ```sql
   \d k8s_custom_resources
   ```
   Expected columns:
   - `_cq_sync_time` (timestamp)
   - `_cq_source_name` (text)
   - `_cq_id` (uuid, not null)
   - `_cq_parent_id` (uuid)
   - `context` (text)
   - `kind` (text)
   - `api_version` (text)
   - `name` (text)
   - `namespace` (text)
   - `uid` (text, not null)
   - `labels` (jsonb)
   - `annotations` (jsonb)
   - `owner_references` (jsonb)
   - `finalizers` (text[])
   - `spec` (jsonb)
   - `status` (jsonb)

5. Query data to verify types:
   ```sql
   SELECT api_version, kind, name, 
          jsonb_pretty(labels), 
          jsonb_pretty(spec), 
          array_length(finalizers, 1)
   FROM k8s_custom_resources;
   ```

**Acceptance**: E2E test successful, PostgreSQL schema matches exactly

---

## Phase 3: Commit and Documentation (30 minutes)

### Task 3.1: Review Changes
**Time**: 10 minutes

**Steps**:
1. Review all changed files
2. Verify no regressions
3. Check test coverage

---

### Task 3.2: Commit Changes
**Time**: 10 minutes

**Steps**:
1. `git add` modified files
2. Commit with message:
   ```
   refactor: Match CloudQuery k8s_custom_resources schema
   
   - Use WithPrimaryKeys("UID") for deterministic _cq_id
   - Split GVK into api_version and kind fields
   - Use map types for JSON columns (labels, annotations, spec, status)
   - Add owner_references and finalizers fields
   - Remove manual column definitions
   - Simplify table definition following crds.go pattern
   - Update documentation with correct primary key info
   
   This brings the implementation in line with CloudQuery's official
   k8s plugin schema for better compatibility.
   ```

---

### Task 3.3: Update Spec Status
**Time**: 10 minutes

**Steps**:
1. Update spec.md status to "Complete"
2. Document any deviations or notes
3. Add test results summary

---

## Task Summary

| Phase | Tasks | Time |
|-------|-------|------|
| Phase 1: Code Updates | 4 tasks | 1.5 hours |
| Phase 2: Testing | 3 tasks | 1 hour |
| Phase 3: Commit | 3 tasks | 30 minutes |
| **Total** | **10 tasks** | **3 hours** |

## Commands Quick Reference

```bash
# Build
cd /Users/prajjwaltawri/Desktop/k8cloudquery/ibcq-source-k8s/ibcq-source-k8s
go build -o k8s-source .

# Run tests
go test ./resources/services/customresources/... -v

# E2E sync
cd /Users/prajjwaltawri/Desktop/k8cloudquery/ibcq-source-k8s
cloudquery sync config.yml

# Check PostgreSQL schema
psql "postgres://postgres:postgres@localhost:5434/k8s?sslmode=disable" \
  -c "\d k8s_custom_resources"

# Verify data and types
psql "postgres://postgres:postgres@localhost:5434/k8s?sslmode=disable" \
  -c "SELECT api_version, kind, name, namespace, uid, 
      pg_typeof(labels) as labels_type,
      pg_typeof(spec) as spec_type,
      pg_typeof(finalizers) as finalizers_type,
      jsonb_pretty(labels::jsonb) as labels_data,
      jsonb_pretty(spec::jsonb) as spec_data
      FROM k8s_custom_resources;"

# Commit
git add -A
git commit -m "refactor: Match CloudQuery k8s_custom_resources schema"
git push origin cq-custom-resource
```

## Next Steps

After completion:
1. Push changes to branch
2. Update PR description
3. Request review from team
