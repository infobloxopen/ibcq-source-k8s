# Implementation Tasks: CloudQuery Compatibility Update

**Spec**: `002-cloudquery-compat`  
**Branch**: `cq-custom-resource`  
**Created**: 2026-03-05  
**Status**: In Progress

## Phase 1: Code Changes ✅ COMPLETE

### ✅ Task 1.1: Simplify Table Definition
**Status**: Complete  
**File**: `resources/services/customresources/custom_resources.go`

Changes made:
- Removed all manual column definitions
- Added `transformers.WithPrimaryKeys("UID")`
- Kept only `client.ContextColumn` in Columns list
- Removed unused imports (json, time, arrow)

---

### ✅ Task 1.2: Update CustomResourceRow Struct
**Status**: Complete  
**File**: `resources/services/customresources/custom_resources.go`

Changes made:
- Replaced `GVK string` with `APIVersion string` and `Kind string`
- Changed `Labels` from `string` to `map[string]string`
- Changed `Annotations` from `string` to `map[string]string`
- Added `OwnerReferences []any`
- Added `Finalizers []string`
- Changed `Spec` from `string` to `map[string]any`
- Changed `Status` from `string` to `map[string]any`
- Removed: `ResourceVersion`, `Generation`, `CreatedAt`

---

### ✅ Task 1.3: Update convertToCustomResourceRow Function
**Status**: Complete  
**File**: `resources/services/customresources/custom_resources.go`

Changes made:
- Removed all JSON marshaling
- Direct map assignments for labels, annotations, spec, status
- Extract and assign owner references
- Extract and assign finalizers
- Set APIVersion and Kind separately

---

### ✅ Task 1.4: Update Documentation
**Status**: Complete  
**File**: `docs/tables/k8s_custom_resources.md`

Changes made:
- Updated primary key section (uid with deterministic _cq_id)
- Updated columns table (removed old fields, added new ones)
- Changed types to JSON where appropriate
- Added PostgreSQL jsonb storage notes

---

## Phase 2: Test Updates 🔄 IN PROGRESS

### 🔄 Task 2.1: Fix Unit Test Compilation Errors
**Status**: In Progress  
**File**: `resources/services/customresources/custom_resources_test.go`

**Remaining Errors**:
```
Line 150: unknown field GVK
Line 154: unknown field ResourceVersion
Line 155: unknown field Generation
Line 156: cannot use string as map[string]string
Line 157: cannot use string as map[string]string
Line 158: unknown field CreatedAt
Line 159: cannot use string as map[string]any
Line 160: cannot use string as map[string]any
Line 165: row.GVK undefined
Line 168: row.Generation undefined
```

**Sub-tasks**:
- [x] Update TestCustomResourceRow_Structure
- [x] Update TestCustomResources_PrimaryKeys
- [x] Update TestConvertToCustomResourceRow first test case
- [ ] Fix remaining test cases in TestConvertToCustomResourceRow
- [ ] Update TestCustomResources_ColumnTypes
- [ ] Update any other failing tests

---

### ⏳ Task 2.2: Update Integration Tests
**Status**: Not Started  
**File**: `resources/services/customresources/integration_test.go`

**Steps**:
1. Update test object creation for new struct fields
2. Change string assertions to map assertions
3. Add tests for owner_references
4. Add tests for finalizers
5. Remove tests for deleted fields

---

### ⏳ Task 2.3: Run Full Test Suite
**Status**: Not Started

**Steps**:
1. Run all unit tests: `go test ./resources/services/customresources/... -v`
2. Verify all tests pass
3. Check test coverage

---

## Phase 3: End-to-End Verification ⏳ NOT STARTED

### ⏳ Task 3.1: Build Plugin
**Status**: Not Started

**Steps**:
1. Run `go build -o k8s-source .`
2. Verify no compilation errors
3. Check binary size is reasonable

---

### ⏳ Task 3.2: Sync Data to PostgreSQL
**Status**: Not Started

**Steps**:
1. Ensure Widget CR is deployed: `kubectl get widgets.example.com -n default`
2. Run sync: `cloudquery sync config.yml`
3. Verify sync completes successfully

---

### ⏳ Task 3.3: Verify PostgreSQL Schema
**Status**: Not Started

**Steps**:
1. Connect to database
2. Run `\d k8s_custom_resources` to check schema
3. Verify columns match expected:
   - _cq_id (uuid, not null)
   - context (text)
   - api_version (text)
   - kind (text)
   - name (text)
   - namespace (text)
   - uid (text, not null)
   - labels (jsonb)
   - annotations (jsonb)
   - owner_references (jsonb)
   - finalizers (text[])
   - spec (jsonb)
   - status (jsonb)

---

### ⏳ Task 3.4: Verify Data Correctness
**Status**: Not Started

**Steps**:
1. Query: `SELECT api_version, kind, name, namespace, uid FROM k8s_custom_resources;`
2. Verify api_version is "example.com/v1alpha1"
3. Verify kind is "Widget"
4. Query JSON fields: `SELECT jsonb_pretty(labels), jsonb_pretty(spec) FROM k8s_custom_resources;`
5. Verify labels and spec are proper JSON
6. Check finalizers type: `SELECT pg_typeof(finalizers) FROM k8s_custom_resources;`
7. Verify it's `text[]`

---

## Phase 4: Commit and Documentation ⏳ NOT STARTED

### ⏳ Task 4.1: Review All Changes
**Status**: Not Started

**Steps**:
1. Review code changes: `git diff`
2. Review test changes
3. Ensure no debug code left
4. Check for any TODOs or FIXMEs

---

### ⏳ Task 4.2: Update Spec Status
**Status**: Not Started

**Steps**:
1. Update `specs/002-cloudquery-compat/spec.md` status to "Complete"
2. Check all success criteria boxes
3. Add any notes or caveats

---

### ⏳ Task 4.3: Commit Changes
**Status**: Not Started

**Steps**:
1. Stage all files: `git add -A`
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
   - Update tests for new struct
   
   PostgreSQL schema now matches CloudQuery official plugin:
   - labels, annotations, owner_references, spec, status as jsonb
   - finalizers as text[]
   - uid as primary key with deterministic _cq_id
   
   All tests passing. End-to-end verified with Widget CR.
   ```
3. Push: `git push origin cq-custom-resource`

---

### ⏳ Task 4.4: Update PR
**Status**: Not Started

**Steps**:
1. Update PR description with CloudQuery compatibility changes
2. Add before/after schema comparison
3. Add verification screenshots/output
4. Request review

---

## Summary

### Completed: 4 tasks
- Task 1.1: Simplify table definition
- Task 1.2: Update struct
- Task 1.3: Update conversion function
- Task 1.4: Update documentation

### In Progress: 1 task
- Task 2.1: Fix unit test compilation (partially done)

### Not Started: 10 tasks
- Task 2.1: Complete unit test fixes
- Task 2.2: Update integration tests
- Task 2.3: Run full test suite
- Task 3.1: Build plugin
- Task 3.2: Sync data
- Task 3.3: Verify schema
- Task 3.4: Verify data
- Task 4.1: Review changes
- Task 4.2: Update spec status
- Task 4.3: Commit
- Task 4.4: Update PR

### Total: 15 tasks (4 done, 1 in progress, 10 remaining)

---

## Next Action

**Current Focus**: Task 2.1 - Fix remaining unit test compilation errors

**Immediate Steps**:
1. Fix all TestConvertToCustomResourceRow test cases
2. Update TestCustomResources_ColumnTypes
3. Run tests to find any other issues
4. Move to integration tests

**Command to run**:
```bash
go test ./resources/services/customresources/... -v
```
