# Implementation Tasks: Custom Resources Support

**Feature**: Add Custom Resources (CR) collection to K8s CloudQuery plugin  
**Branch**: `cq-crd`  
**Created**: 2026-02-27  
**Status**: Ready for Implementation

This document breaks down the implementation plan into specific, actionable tasks organized by phase.

---

## Phase 1: Foundation (Priority P0)

### Task 1.1: Extend Spec Configuration Schema
**Estimated Time**: 2 hours  
**Dependencies**: None  
**Files**:
- `client/spec/spec.go`
- `client/spec/schema.json`

**Steps**:
1. Add `CustomResourceSpec` struct to `spec.go`:
   ```go
   type CustomResourceSpec struct {
       GVK        string   `yaml:"gvk" json:"gvk" jsonschema:"required"`
       Namespaces []string `yaml:"namespaces,omitempty" json:"namespaces"`
   }
   ```
2. Add `CustomResources []CustomResourceSpec` field to `Spec` struct
3. Update `schema.json` with new fields and JSON schema validation
4. Test: Verify JSON schema validation accepts valid configs

**Acceptance**:
- [ ] Configuration parses correctly from YAML
- [ ] JSON schema validates GVK format
- [ ] Invalid configs are rejected with clear error messages

---

### Task 1.2: Add Configuration Validation
**Estimated Time**: 2 hours  
**Dependencies**: Task 1.1  
**Files**:
- `client/spec/spec.go`
- `client/spec/spec_test.go`

**Steps**:
1. Implement `Validate()` method on `Spec`:
   - Validate GVK format: `group/version/kind`
   - Check group is not empty
   - Check version is not empty
   - Check kind starts with uppercase letter
2. Implement `validateGVK(string) error` helper function
3. Write unit tests for validation:
   - Valid GVK formats
   - Invalid formats (missing parts, lowercase kind, etc.)
   - Edge cases (empty fields, special characters)

**Acceptance**:
- [ ] All validation tests pass
- [ ] Clear error messages for each validation failure
- [ ] Validation called during plugin initialization

---

### Task 1.3: Write Spec Validation Tests
**Estimated Time**: 1 hour  
**Dependencies**: Task 1.2  
**Files**:
- `client/spec/spec_test.go`

**Steps**:
1. Test valid configurations:
   - Single CR: `cert-manager.io/v1/Certificate`
   - Multiple CRs with different groups
   - With namespace filters
   - Empty CR list (valid)
2. Test invalid configurations:
   - Invalid GVK format: `cert-manager/Certificate` (missing version)
   - Lowercase kind: `cert-manager.io/v1/certificate`
   - Empty group or version
   - Special characters in group name
3. Run tests and verify all pass

**Acceptance**:
- [ ] 100% test coverage for validation logic
- [ ] Tests cover all edge cases
- [ ] All tests pass

---

## Phase 2: Client Integration (Priority P0)

### Task 2.1: Add Dynamic Client to Client Struct
**Estimated Time**: 2 hours  
**Dependencies**: Phase 1 complete  
**Files**:
- `client/client.go`

**Steps**:
1. Add import: `"k8s.io/client-go/dynamic"`
2. Add field to `Client` struct:
   ```go
   dynamicClients map[string]dynamic.Interface  // context -> dynamic client
   ```
3. Add accessor method:
   ```go
   func (c *Client) DynamicClient() dynamic.Interface {
       return c.dynamicClients[c.Context]
   }
   ```
4. Export `Spec()` accessor if not already available:
   ```go
   func (c *Client) Spec() *spec.Spec {
       return c.spec
   }
   ```

**Acceptance**:
- [ ] `Client` struct has `dynamicClients` field
- [ ] `DynamicClient()` method returns correct client for context
- [ ] Code compiles without errors

---

### Task 2.2: Initialize Dynamic Clients in Configure()
**Estimated Time**: 2 hours  
**Dependencies**: Task 2.1  
**Files**:
- `client/client.go`

**Steps**:
1. Locate `Configure()` function
2. Find where `rest.Config` is created for each context
3. Add dynamic client initialization:
   ```go
   dynamicClients := make(map[string]dynamic.Interface)
   for ctxName, restConfig := range restConfigs {
       dynClient, err := dynamic.NewForConfig(restConfig)
       if err != nil {
           return nil, fmt.Errorf("failed to create dynamic client for context %s: %w", ctxName, err)
       }
       dynamicClients[ctxName] = dynClient
   }
   ```
4. Pass `dynamicClients` to `Client` initialization
5. Call `spec.Validate()` before creating clients

**Acceptance**:
- [ ] Dynamic clients initialized for all contexts
- [ ] Error handling for client creation failures
- [ ] Configuration validation runs before client creation
- [ ] Multi-context support works correctly

---

### Task 2.3: Write Client Initialization Tests
**Estimated Time**: 2 hours  
**Dependencies**: Task 2.2  
**Files**:
- `client/client_test.go`

**Steps**:
1. Test successful client initialization with CR config
2. Test multiple contexts with dynamic clients
3. Test error handling for invalid config
4. Test `DynamicClient()` accessor returns correct client
5. Verify existing tests still pass

**Acceptance**:
- [ ] All new tests pass
- [ ] No regression in existing tests
- [ ] Mock-based tests use proper gomock patterns

---

## Phase 3: Table Definition (Priority P1)

### Task 3.1: Create Custom Resources Package
**Estimated Time**: 1 hour  
**Dependencies**: Phase 2 complete  
**Files**:
- `resources/services/customresources/` (new directory)
- `resources/services/customresources/custom_resources.go`

**Steps**:
1. Create directory: `resources/services/customresources/`
2. Create file: `custom_resources.go`
3. Add package declaration and imports
4. Create placeholder `CustomResources()` function returning `*schema.Table`

**Acceptance**:
- [ ] Package structure created
- [ ] File compiles successfully
- [ ] Package follows existing service patterns

---

### Task 3.2: Define CustomResourceRow Struct
**Estimated Time**: 1 hour  
**Dependencies**: Task 3.1  
**Files**:
- `resources/services/customresources/custom_resources.go`

**Steps**:
1. Define `CustomResourceRow` struct with fields:
   - `APIVersion string`
   - `Kind string`
   - `Metadata` struct (name, namespace, uid, labels, annotations, etc.)
   - `Spec map[string]interface{}`
   - `Status map[string]interface{}`
2. Match field names to CloudQuery closed-source plugin schema
3. Add JSON tags for proper serialization

**Acceptance**:
- [ ] Struct fields match required schema
- [ ] Proper Go naming conventions
- [ ] JSON/YAML tags correct

---

### Task 3.3: Implement Table Schema with Columns
**Estimated Time**: 3 hours  
**Dependencies**: Task 3.2  
**Files**:
- `resources/services/customresources/custom_resources.go`

**Steps**:
1. Implement `CustomResources()` function returning `*schema.Table`
2. Define table properties:
   - Name: `"k8s_custom_resources"`
   - Resolver: `fetchCustomResources` (placeholder)
   - Multiplex: `customResourceMultiplex` (placeholder)
   - Transform: use `transformers.TransformWithStruct(&CustomResourceRow{})`
   - Primary key: `UID`
3. Define columns:
   - `context` (from `client.ContextColumn`)
   - `api_version` (String)
   - `kind` (String)
   - `name`, `namespace`, `uid` (String)
   - `labels`, `annotations` (JSON String)
   - `owner_references` (JSON String)
   - `finalizers` (List<String>)
   - `spec` (JSON String)
   - `status` (JSON String)
   - `creation_timestamp` (Timestamp)
4. Add descriptions to each column

**Acceptance**:
- [ ] Table schema defined correctly
- [ ] All required columns present
- [ ] Column types match Arrow types
- [ ] Schema compatible with closed-source plugin

---

### Task 3.4: Implement JSON Resolver Helpers
**Estimated Time**: 2 hours  
**Dependencies**: Task 3.3  
**Files**:
- `resources/services/customresources/helpers.go` (new file)

**Steps**:
1. Create `helpers.go` file
2. Implement `jsonResolver(path string) schema.ColumnResolver`:
   - Accepts field path (e.g., "Spec", "Status")
   - Marshals value to JSON string
   - Returns nil for empty values
3. Implement `jsonMapResolver(path string) schema.ColumnResolver`:
   - For map[string]string fields (labels, annotations)
   - Marshals to JSON string
4. Add error handling for JSON marshaling failures
5. Write unit tests for resolvers

**Acceptance**:
- [ ] JSON resolvers work with test data
- [ ] Nil/empty values handled correctly
- [ ] Unit tests pass
- [ ] Error handling covers edge cases

---

## Phase 4: Fetcher Implementation (Priority P1)

### Task 4.1: Implement Resource Pluralization
**Estimated Time**: 2 hours  
**Dependencies**: Phase 3 complete  
**Files**:
- `resources/services/customresources/helpers.go`

**Steps**:
1. Implement `pluralize(kind string) string`:
   - Basic rules: add "s"
   - Special cases: "y" → "ies", "s" → "ses"
   - Return lowercase
2. Add test cases:
   - Certificate → certificates
   - Deployment → deployments
   - Ingress → ingresses
   - Proxy → proxies
3. TODO comment: Consider fetching from CRD definition

**Acceptance**:
- [ ] Common K8s resource names pluralized correctly
- [ ] Unit tests cover edge cases
- [ ] Fallback logic for unknown patterns

---

### Task 4.2: Implement GVK Parsing
**Estimated Time**: 1 hour  
**Dependencies**: Task 4.1  
**Files**:
- `resources/services/customresources/helpers.go`

**Steps**:
1. Implement `parseGVK(gvkStr string) schema.GroupVersionKind`:
   - Split string by "/"
   - Return GroupVersionKind struct
   - Handle validation (already done in spec validation)
2. Write unit tests for parsing:
   - Valid formats
   - Various group/version/kind combinations

**Acceptance**:
- [ ] GVK parsed correctly
- [ ] Unit tests pass
- [ ] No validation needed (handled in spec)

---

### Task 4.3: Implement Unstructured to Row Conversion
**Estimated Time**: 2 hours  
**Dependencies**: Task 4.2  
**Files**:
- `resources/services/customresources/custom_resources.go`

**Steps**:
1. Implement `convertUnstructuredToRow(u *unstructured.Unstructured) *CustomResourceRow`:
   - Extract APIVersion, Kind from unstructured
   - Extract metadata fields (name, namespace, uid, labels, etc.)
   - Extract spec as map[string]interface{}
   - Extract status as map[string]interface{}
   - Handle missing fields gracefully (spec/status may not exist)
2. Write unit tests with sample unstructured objects
3. Test edge cases: missing spec, missing status, empty metadata

**Acceptance**:
- [ ] Conversion handles all fields correctly
- [ ] Missing fields don't cause panics
- [ ] Unit tests pass
- [ ] Edge cases covered

---

### Task 4.4: Implement Custom Resource Multiplex
**Estimated Time**: 3 hours  
**Dependencies**: Task 4.3  
**Files**:
- `resources/services/customresources/custom_resources.go`

**Steps**:
1. Define `customResourceClient` struct:
   ```go
   type customResourceClient struct {
       *client.Client
       gvk      schema.GroupVersionKind
       nsFilter []string
   }
   ```
2. Implement `customResourceMultiplex(ctx, meta, parent) []schema.ClientMeta`:
   - Get custom resources from spec: `cl.Spec().CustomResources`
   - Return empty slice if no CRs configured
   - Loop through contexts and GVKs
   - Create `customResourceClient` for each context+GVK combination
   - Return slice of clients
3. Write unit tests for multiplex logic

**Acceptance**:
- [ ] Multiplexing creates correct client combinations
- [ ] Empty config returns empty slice
- [ ] Multiple contexts and GVKs handled correctly
- [ ] Unit tests pass

---

### Task 4.5: Implement Custom Resource Fetcher
**Estimated Time**: 4 hours  
**Dependencies**: Task 4.4  
**Files**:
- `resources/services/customresources/custom_resources.go`

**Steps**:
1. Implement `fetchCustomResources(ctx, meta, parent, res) error`:
   - Cast meta to `*customResourceClient`
   - Get dynamic client: `crClient.DynamicClient()`
   - Create GVR (GroupVersionResource) from GVK
   - Determine namespaces to query:
     - If `nsFilter` is empty, use all namespaces + cluster-scoped
     - Otherwise, use specified namespaces
   - Loop through namespaces:
     - Get resource interface (namespaced or cluster-scoped)
     - List resources with pagination (Continue token)
     - Convert each unstructured item to `CustomResourceRow`
     - Send rows to `res` channel
   - Error handling: log errors but continue with other resources
2. Handle pagination correctly with Continue token
3. Add logging for errors and progress

**Acceptance**:
- [ ] Fetcher retrieves CRs from K8s API
- [ ] Pagination works correctly
- [ ] Namespace filtering works
- [ ] Error handling doesn't stop entire sync
- [ ] Logging provides useful debugging info

---

### Task 4.6: Write Fetcher Unit Tests
**Estimated Time**: 3 hours  
**Dependencies**: Task 4.5  
**Files**:
- `resources/services/customresources/custom_resources_test.go` (new file)

**Steps**:
1. Generate mocks for dynamic client interfaces
2. Create test helper to build mock dynamic client
3. Write tests:
   - Test successful CR fetch
   - Test pagination with Continue token
   - Test namespace filtering
   - Test multiple GVKs
   - Test error handling (missing CRD, permission denied)
   - Test empty result set
4. Use gomock for mock expectations
5. Verify all code paths covered

**Acceptance**:
- [ ] All unit tests pass
- [ ] Mocks properly configured with gomock
- [ ] Test coverage > 80%
- [ ] Edge cases tested

---

## Phase 5: Integration & Testing (Priority P1)

### Task 5.1: Register Table in Plugin
**Estimated Time**: 30 minutes  
**Dependencies**: Phase 4 complete  
**Files**:
- `resources/plugin/plugin.go`

**Steps**:
1. Add import: `"github.com/cloudquery/cloudquery/plugins/source/k8s/resources/services/customresources"`
2. Add table to `getTables()` function:
   ```go
   customresources.CustomResources(),
   ```
3. Verify plugin compiles

**Acceptance**:
- [ ] Table registered in plugin
- [ ] Plugin compiles successfully
- [ ] No import conflicts

---

### Task 5.2: Generate Mocks for Dynamic Client
**Estimated Time**: 1 hour  
**Dependencies**: Task 5.1  
**Files**:
- `mockgen.go`
- `mocks/dynamic/` (new directory)

**Steps**:
1. Add mockgen directive to `mockgen.go`:
   ```go
   //go:generate mockgen -package=mocks -destination=./mocks/dynamic/dynamic.go k8s.io/client-go/dynamic Interface,ResourceInterface,NamespaceableResourceInterface
   ```
2. Run: `go generate ./...`
3. Verify mocks generated correctly
4. Commit generated mocks

**Acceptance**:
- [ ] Mocks generated successfully
- [ ] Mock files compile
- [ ] Mocks usable in tests

---

### Task 5.3: Create Integration Test Setup
**Estimated Time**: 2 hours  
**Dependencies**: Task 5.2  
**Files**:
- `resources/services/customresources/integration_test.go` (new file)
- `.github/workflows/test.yml` (if applicable)

**Steps**:
1. Create integration test file with build tag:
   ```go
   // +build integration
   ```
2. Set up test K8s cluster (kind/minikube)
3. Install test CRDs (cert-manager or simple test CRD)
4. Create test configuration with CR specification
5. Write basic integration test:
   - Initialize plugin with test config
   - Sync custom resources table
   - Verify CRs fetched correctly
6. Document how to run integration tests in README

**Acceptance**:
- [ ] Integration test runs successfully locally
- [ ] Test CRDs deployed correctly
- [ ] Instructions documented
- [ ] CI configuration updated (if applicable)

---

### Task 5.4: End-to-End Testing
**Estimated Time**: 3 hours  
**Dependencies**: Task 5.3  
**Files**:
- `resources/services/customresources/integration_test.go`

**Steps**:
1. Test with multiple CR types:
   - cert-manager Certificates
   - ArgoCD Applications
   - Custom test CRD
2. Test with different configurations:
   - Single CR type
   - Multiple CR types
   - With namespace filtering
   - All namespaces
3. Verify data in destination:
   - Correct number of rows
   - All columns populated correctly
   - JSON fields parse correctly
   - Primary key unique
4. Test error scenarios:
   - Non-existent CRD
   - Permission denied
   - Invalid configuration

**Acceptance**:
- [ ] All integration tests pass
- [ ] Data matches expected schema
- [ ] Error scenarios handled gracefully
- [ ] Performance acceptable (< 5s for 100 CRs)

---

### Task 5.5: Performance Testing
**Estimated Time**: 2 hours  
**Dependencies**: Task 5.4  
**Files**:
- `resources/services/customresources/performance_test.go` (new file)

**Steps**:
1. Create performance test with large CR sets:
   - 1,000 CRs
   - 10,000 CRs (if feasible)
2. Measure:
   - Sync time
   - Memory usage
   - CPU usage
3. Test pagination performance
4. Test concurrent fetching (multiple GVKs)
5. Document performance characteristics

**Acceptance**:
- [ ] Performance benchmarks established
- [ ] Memory usage reasonable (< 500MB for 10K CRs)
- [ ] Sync time acceptable (< 60s for 10K CRs)
- [ ] No memory leaks detected

---

## Phase 6: Documentation (Priority P2)

### Task 6.1: Create Table Documentation
**Estimated Time**: 2 hours  
**Dependencies**: Phase 5 complete  
**Files**:
- `docs/tables/k8s_custom_resources.md` (new file)

**Steps**:
1. Create markdown documentation file
2. Document table schema:
   - Table name and description
   - Column list with types and descriptions
   - Primary key information
3. Add configuration examples:
   - Basic example with single CR
   - Multiple CRs
   - With namespace filtering
4. Add SQL query examples:
   - Query all CRs
   - Filter by kind
   - Extract spec fields with JSON operators
   - Join with other K8s tables
5. Follow format of existing table docs

**Acceptance**:
- [ ] Documentation complete and accurate
- [ ] Examples tested and working
- [ ] Follows existing documentation style
- [ ] SQL examples execute correctly

---

### Task 6.2: Update Configuration Documentation
**Estimated Time**: 1 hour  
**Dependencies**: Task 6.1  
**Files**:
- `docs/_configuration.md`
- `docs/configuration.md`

**Steps**:
1. Add section on `custom_resources` configuration:
   - Explain GVK format
   - Explain namespace filtering
   - Provide examples
2. Add notes on:
   - How to find GVK for installed CRDs
   - Performance considerations
   - Limitations (no wildcard support in Phase 1)
3. Update configuration schema documentation

**Acceptance**:
- [ ] Configuration section added
- [ ] Clear examples provided
- [ ] Limitations documented
- [ ] Integrated with existing docs

---

### Task 6.3: Update Main README
**Estimated Time**: 30 minutes  
**Dependencies**: Task 6.2  
**Files**:
- `README.md`

**Steps**:
1. Add feature description to README:
   - Mention custom resources support
   - Link to table documentation
   - Link to configuration docs
2. Update features list
3. Add quick example in README

**Acceptance**:
- [ ] README updated
- [ ] Links working correctly
- [ ] Example clear and concise

---

### Task 6.4: Create Quickstart Guide
**Estimated Time**: 1 hour  
**Dependencies**: Task 6.3  
**Files**:
- `specs/001-cq-crd/quickstart.md`

**Steps**:
1. Write step-by-step quickstart:
   - Prerequisites (K8s cluster, CRDs installed)
   - Installation steps
   - Configuration example
   - Running sync
   - Querying results
2. Add troubleshooting section:
   - Common errors
   - How to debug
   - Permission issues

**Acceptance**:
- [ ] Quickstart guide complete
- [ ] Steps tested and working
- [ ] Covers common use cases
- [ ] Troubleshooting helpful

---

## Phase 7: Advanced Features (Priority P3) - DEFERRED

These tasks are for future enhancements and not part of the initial implementation:

### Task 7.1: Wildcard Pattern Matching
- Implement wildcard support for group: `*.cert-manager.io/v1/*`
- Implement wildcard for version: `cert-manager.io/*/Certificate`
- Implement wildcard for kind: `cert-manager.io/v1/*`

### Task 7.2: Exclude Patterns
- Add `exclude` field to configuration
- Implement exclusion logic in multiplex/fetcher
- Document exclude patterns

### Task 7.3: CRD Auto-Discovery
- Add `discover_all` configuration option
- Fetch all CRDs from cluster
- Filter based on include/exclude patterns

### Task 7.4: Intelligent Resource Name Resolution
- Fetch resource names from CRD definitions
- Use CRD plural name instead of pluralization logic
- Cache CRD definitions for performance

---

## Testing Checklist

Before marking implementation complete, verify:

- [ ] All unit tests pass
- [ ] All integration tests pass
- [ ] Test coverage > 80%
- [ ] No regression in existing functionality
- [ ] Performance tests pass
- [ ] Documentation complete and accurate
- [ ] Examples tested and working
- [ ] Code follows existing patterns
- [ ] No linting errors
- [ ] Mocks generated and committed
- [ ] Configuration schema updated
- [ ] Table registered in plugin

---

## Definition of Done

A task is complete when:
1. ✅ Code implemented according to specification
2. ✅ Unit tests written and passing
3. ✅ Integration tests passing (if applicable)
4. ✅ Documentation updated
5. ✅ Code reviewed (self-review at minimum)
6. ✅ No known bugs or issues
7. ✅ Committed to feature branch

---

## Task Summary

| Phase | Tasks | Estimated Time |
|-------|-------|----------------|
| Phase 1: Foundation | 3 tasks | 5 hours |
| Phase 2: Client Integration | 3 tasks | 6 hours |
| Phase 3: Table Definition | 4 tasks | 7 hours |
| Phase 4: Fetcher Implementation | 6 tasks | 15 hours |
| Phase 5: Integration & Testing | 5 tasks | 11.5 hours |
| Phase 6: Documentation | 4 tasks | 4.5 hours |
| **Total** | **25 tasks** | **49 hours** |

**Note**: Phase 7 (Advanced Features) is deferred to future iterations.

---

## Next Steps

1. Review and approve this task breakdown
2. Set up development environment
3. Create feature branch: `cq-crd`
4. Begin with Phase 1, Task 1.1
5. Follow test-driven development: write tests before implementation
6. Commit frequently with clear messages
7. Update task status as you progress
