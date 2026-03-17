# Task Checklist: Concurrent GVK Processing (003)

**Feature**: Concurrent GVK Processing  
**Branch**: `003-concurrent-gvk-processing`  
**Created**: 2026-03-17  
**Status**: Planning Complete, Ready for Phase 1 Implementation  

---

## Overview

This checklist tracks all tasks across 4 implementation phases. Each phase has clear deliverables, validation criteria, and success metrics.

**Effort Estimate**: 12-16 hours  
**Timeline**: 2-3 days (assumes ~6-8 hrs/day)  
**Success Criteria**: 5-10x faster multi-GVK sync, partial failure resilience, all tests passing

---

## PHASE 1: Concurrent Fetcher Core (4-6 hours)

### Setup & Planning
- [ ] **Setup-001**: Create feature branch `003-concurrent-gvk-processing`
  - Command: `git checkout -b 003-concurrent-gvk-processing`
  - Verify: `git branch | grep 003`
  - Owner: Dev
  - Effort: 5 min

- [ ] **Setup-002**: Create `specs/003-concurrent-gvk/` directory structure
  - Command: `mkdir -p ibcq-source-k8s/specs/003-concurrent-gvk`
  - Verify: Directory exists
  - Owner: Dev
  - Effort: 2 min

- [ ] **Setup-003**: Create spec.md (Feature Specification)
  - File: `specs/003-concurrent-gvk/spec.md`
  - Deliverable: User scenarios, requirements, success criteria
  - Owner: Dev
  - Effort: 1 hour
  - Status: ✅ COMPLETE

- [ ] **Setup-004**: Create plan.md (Implementation Plan)
  - File: `specs/003-concurrent-gvk/plan.md`
  - Deliverable: Phase breakdown, code sketches, validation strategy
  - Owner: Dev
  - Effort: 1 hour
  - Status: ✅ COMPLETE

- [ ] **Setup-005**: Create TASKS.md (This checklist)
  - File: `specs/003-concurrent-gvk/TASKS.md`
  - Deliverable: Detailed task list with effort estimates
  - Owner: Dev
  - Effort: 30 min
  - Status: ✅ IN PROGRESS

### Code Implementation
- [ ] **Coding-101**: Create `config.go` with ConcurrencyConfig
  - File: `ibcq-source-k8s/resources/services/customresources/config.go`
  - Code: ConcurrencyConfig struct, DefaultConcurrencyConfig()
  - Tests: Unit test for default values
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Compiles, test passes

- [ ] **Coding-102**: Create `types.go` with FetchResult
  - File: `ibcq-source-k8s/resources/services/customresources/types.go`
  - Code: FetchResult struct (GVK, Resources, Error, Duration, Attempts)
  - Tests: Unit test for FetchResult construction
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Compiles, JSON marshaling works

- [ ] **Coding-103**: Refactor fetchCustomResources() to concurrent version
  - File: `ibcq-source-k8s/resources/services/customresources/custom_resources.go`
  - Changes:
    - Replace `for` loop with `errgroup.WithContext()`
    - Use `g.SetLimit(config.MaxConcurrentGVKs)`
    - Spawn goroutine per GVK
    - Collect results in mutex-protected slice
    - Return combined rows
  - Tests: Existing tests still pass
  - Owner: Dev
  - Effort: 1.5 hours
  - Acceptance: `go test ./...` passes all existing tests

- [ ] **Coding-104**: Verify dynamic client reusability
  - File: `ibcq-source-k8s/client/client.go`
  - Review: Confirm dynamicClient is thread-safe
  - Tests: Add comment if already safe
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: No changes needed or documented

- [ ] **Coding-105**: Ensure imports (golang.org/x/sync)
  - Command: `go get golang.org/x/sync@latest`
  - Verify: `go mod tidy && go build ./...`
  - Owner: Dev
  - Effort: 10 min
  - Acceptance: Build succeeds

### Validation (Phase 1)
- [ ] **Valid-101**: Run existing test suite
  - Command: `go test -v ./resources/services/customresources/...`
  - Acceptance: All 19 existing tests pass
  - Owner: Dev
  - Effort: 5 min

- [ ] **Valid-102**: Write TestConcurrentFetch_MultipleGVKs
  - File: `custom_resources_test.go`
  - Test: 5 GVKs, all complete without error
  - Assertions: 5 results, no error, resource count > 0
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Test passes

- [ ] **Valid-103**: Write TestConcurrentFetch_BoundedQueue
  - File: `custom_resources_test.go`
  - Test: 10 GVKs, concurrency limit = 2
  - Assertions: Max 2 goroutines running simultaneously
  - Owner: Dev
  - Effort: 45 min (needs timing instrumentation)
  - Acceptance: Test passes, proves bounded concurrency

- [ ] **Valid-104**: Write BenchmarkConcurrentVsSequential
  - File: `custom_resources_test.go` or `benchmark_test.go`
  - Test: 10 GVKs × 100 resources, measure time
  - Assertions: Concurrent ≤ 1.5x slowest GVK, not sum
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Benchmark runs, shows 3-5x speedup

- [ ] **Valid-105**: Manual smoke test
  - Command: Build and run against real cluster
  - Verify: All GVKs fetch successfully
  - Owner: Dev
  - Effort: 15 min
  - Acceptance: Sync completes, no errors

### Commit & Documentation
- [ ] **Commit-101**: Commit Phase 1 implementation
  - Message: "feat(concurrent): Add concurrent GVK fetching with errgroup"
  - Files: config.go, types.go, custom_resources.go (updated), updates to tests
  - Owner: Dev
  - Effort: 10 min

- [ ] **Commit-102**: Commit Phase 1 tests
  - Message: "test(concurrent): Add concurrency unit tests and benchmarks"
  - Files: benchmark_test.go, updated custom_resources_test.go
  - Owner: Dev
  - Effort: 5 min

- [ ] **Doc-101**: Add inline comments explaining goroutine logic
  - File: custom_resources.go
  - Explanation: Why errgroup, how concurrency is bounded, error handling
  - Owner: Dev
  - Effort: 15 min

---

## PHASE 2: Error Resilience (2-3 hours)

### Code Implementation
- [ ] **Coding-201**: Create GVKError type
  - File: `custom_resources.go` (add to types)
  - Fields: GVK, Err, Count
  - Owner: Dev
  - Effort: 10 min
  - Acceptance: Compiles

- [ ] **Coding-202**: Refactor error handling to collect per-GVK errors
  - File: `custom_resources.go`
  - Changes:
    - Create `errors []GVKError` slice
    - Append to errors instead of returning immediately
    - Check `successCount > 0` for overall success
    - Log summary at end
  - Owner: Dev
  - Effort: 40 min
  - Acceptance: Function signature unchanged, error behavior different

- [ ] **Coding-203**: Add per-GVK timing and status logs
  - File: `custom_resources.go`
  - Changes:
    - Track start time per GVK
    - Log duration, resource count, status
    - Use structured logging (JSON)
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Logs contain GVK, duration, status for each

### Validation (Phase 2)
- [ ] **Valid-201**: Write TestConcurrentFetch_PartialFailure
  - File: `custom_resources_test.go`
  - Test: 3 GVKs, mock 1 to fail (e.g., RBAC error)
  - Assertions:
    - Other 2 GVKs succeed
    - Error logged but sync continues
    - Partial success (2 GVKs) is acceptable
  - Owner: Dev
  - Effort: 45 min
  - Acceptance: Test passes, proves partial success

- [ ] **Valid-202**: Write TestErrorAggregation
  - File: `custom_resources_test.go`
  - Test: Multiple GVK errors aggregated
  - Assertions:
    - All errors collected
    - All errors logged
    - Summary shows failures
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Test passes

- [ ] **Valid-203**: Manual test: One GVK fails, others succeed
  - Setup: 3 GVK config, RBAC deny for 1
  - Action: Run sync
  - Verify:
    - Error logged for failing GVK
    - Other 2 GVKs store resources
    - Sync completes (exit code 0)
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Sync completes with partial results

- [ ] **Valid-204**: Verify log observability
  - Action: Run sync, capture logs
  - Check: Per-GVK duration, status, resource count visible
  - Owner: Dev
  - Effort: 10 min
  - Acceptance: Logs are clear and queryable

- [ ] **Valid-205**: Create metrics.go and test metrics collection
  - File: Create `metrics.go` with GVKFetchMetrics and SyncSummary types
  - Test: `TestMetricsCollection` verifies timing data captured
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Metrics types compile, test passes

- [ ] **Valid-206**: Verify sync summary report generation
  - Action: Run sync with 3-5 GVKs, check output
  - Verify:
    - Summary table printed with GVK results
    - Per-GVK duration, status, resource count shown
    - Aggregate stats (total resources, speedup, throughput)
    - Failed GVKs listed with error details
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Summary is clear and complete

### Commit & Documentation
- [ ] **Commit-201**: Commit Phase 2 implementation
  - Message: "feat(003/concurrent): Add error aggregation, metrics, and observability"
  - Files: custom_resources.go (updated), metrics.go (new), GVKError type
  - Owner: Dev
  - Effort: 5 min

- [ ] **Commit-202**: Commit Phase 2 tests
  - Message: "test(003/concurrent): Add partial failure, aggregation, and metrics tests"
  - Files: custom_resources_test.go (updated), metrics_test.go (new)
  - Owner: Dev
  - Effort: 5 min

---

## PHASE 3: Rate Limiting & Backoff (2-3 hours)

### Code Implementation
- [ ] **Coding-301**: Create retry.go with IsTransientError
  - File: `ibcq-source-k8s/resources/services/customresources/retry.go`
  - Functions:
    - IsTransientError(err) → bool
    - Checks: timeout, 429, 503, 504
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Compiles, test coverage ≥80%

- [ ] **Coding-302**: Create WithRetry wrapper
  - File: `retry.go` (continuation)
  - Function: WithRetry(ctx, config, fn) → error
  - Features:
    - Exponential backoff (100ms → 10s)
    - Transient vs permanent error handling
    - Context cancellation support
  - Owner: Dev
  - Effort: 45 min
  - Acceptance: Compiles, test coverage ≥80%

- [ ] **Coding-303**: Integrate retry into concurrent fetch
  - File: `custom_resources.go`
  - Changes: Wrap `fetchGVKResources()` call with `WithRetry()`
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Function behavior unchanged for non-transient errors

- [ ] **Coding-304**: Optional: Add dynamic concurrency tuning
  - File: `config.go`
  - Function: AdjustConcurrencyForCluster(nodeCount) → int
  - Logic: Scale 1 worker per 50 nodes (min 5, max 50)
  - Owner: Dev (Optional)
  - Effort: 30 min
  - Acceptance: Unit test verifies scaling

### Validation (Phase 3)
- [ ] **Valid-301**: Write TestRetry_TransientError
  - File: `retry_test.go`
  - Test: Mock timeout on first attempt, success on second
  - Assertions:
    - Retry triggered
    - Backoff delay observed
    - Success after retry
  - Owner: Dev
  - Effort: 40 min
  - Acceptance: Test passes

- [ ] **Valid-302**: Write TestRetry_PermanentError
  - File: `retry_test.go`
  - Test: Mock RBAC error (permanent)
  - Assertions:
    - No retry attempted
    - Error returned immediately
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Test passes

- [ ] **Valid-303**: Write TestBackoffTiming
  - File: `retry_test.go`
  - Test: Verify exponential backoff: 100ms, 200ms, 400ms...
  - Assertions: Backoff increases exponentially, caps at 10s
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Test passes with timing assertions

- [ ] **Valid-304**: Manual test: Transient error recovery
  - Setup: Mock API to timeout once, then succeed
  - Action: Run concurrent fetch
  - Verify:
    - Timeout triggers retry
    - Backoff delay observed
    - Second attempt succeeds
    - Resource stored
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Sync recovers from transient error

### Commit & Documentation
- [ ] **Commit-301**: Commit Phase 3 implementation
  - Message: "feat(concurrent): Add exponential backoff and retry logic"
  - Files: retry.go, custom_resources.go (updated)
  - Owner: Dev
  - Effort: 5 min

- [ ] **Commit-302**: Commit Phase 3 tests
  - Message: "test(concurrent): Add retry and backoff tests"
  - Files: retry_test.go
  - Owner: Dev
  - Effort: 5 min

---

## PHASE 4: Testing & Validation (3-4 hours)

### Unit Tests (New)
- [ ] **Test-401**: Comprehensive unit test suite
  - File: `custom_resources_test.go`
  - Tests: All concurrent scenarios, error cases, timing
  - Count: ≥12 new tests
  - Owner: Dev
  - Effort: 1.5 hours
  - Acceptance: All tests pass, coverage ≥85%

- [ ] **Test-402**: Concurrent memory profiling
  - File: `benchmark_test.go`
  - Test: BenchmarkMemoryOverhead
  - Measure: Memory before/after concurrent fetch
  - Acceptance: Overhead ≤20%
  - Owner: Dev
  - Effort: 30 min

### Benchmarks
- [ ] **Bench-401**: BenchmarkConcurrentVsSequential
  - File: `benchmark_test.go`
  - Setup: 10 GVKs × 100 resources
  - Measure: Concurrent time vs sequential
  - Acceptance: Concurrent ≥3x faster
  - Owner: Dev
  - Effort: 30 min
  - Command: `go test -bench=ConcurrentVsSequential -benchtime=3s`

- [ ] **Bench-402**: BenchmarkConcurrentScaling
  - File: `benchmark_test.go`
  - Setup: Vary GVK count (1, 5, 10, 50)
  - Measure: Time per GVK count
  - Acceptance: Linear until concurrency limit, flat after
  - Owner: Dev
  - Effort: 30 min

- [ ] **Bench-403**: BenchmarkRetryOverhead
  - File: `benchmark_test.go`
  - Setup: Concurrent fetch with/without transient errors
  - Measure: Time difference
  - Acceptance: Retry overhead <10% if no errors
  - Owner: Dev
  - Effort: 20 min

### Integration Tests
- [ ] **Integ-401**: TestIntegration_ConcurrentCustomResourceSync
  - File: `integration_test.go`
  - Setup: Real cluster, 5 GVK configs
  - Action: Concurrent fetch all
  - Assertions:
    - All 5 GVKs complete
    - All resources retrieved
    - Timing shows concurrency
  - Owner: Dev
  - Effort: 45 min
  - Acceptance: Test passes against minikube

- [ ] **Integ-402**: TestIntegration_PartialFailureRecovery
  - File: `integration_test.go`
  - Setup: 3 GVKs, RBAC deny for 1
  - Action: Concurrent fetch all
  - Assertions:
    - 2 GVKs complete
    - Error logged for 1
    - Partial sync succeeds
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Test passes

- [ ] **Integ-403**: TestIntegration_PostgreSQLWrite
  - File: `integration_test.go`
  - Setup: Real PostgreSQL, 3 GVK configs
  - Action: Concurrent fetch and write
  - Assertions:
    - All rows in database
    - Schema matches spec
    - Count matches fetch
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Test passes

### E2E Validation
- [ ] **E2E-401**: Create e2e_concurrent_test.sh script
  - File: `test/e2e_concurrent_test.sh`
  - Setup: Minikube cluster with 5 CRs
  - Action: Full sync with config.yml
  - Verify:
    - All 5 GVKs fetched
    - All resources in PostgreSQL
    - Timing shows concurrency
    - Logs clear and queryable
  - Owner: Dev
  - Effort: 45 min
  - Acceptance: Script runs successfully, all checks pass

- [ ] **E2E-402**: Run E2E script against real cluster
  - Setup: Minikube running, PostgreSQL ready
  - Command: `bash test/e2e_concurrent_test.sh`
  - Verify: All assertions pass
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Script completes with 0 exit code

### Full Regression Test
- [ ] **Regress-401**: Run full test suite
  - Command: `go test -v ./...`
  - Count expected: 19 existing + 15+ new = 34+ tests
  - Acceptance: All tests pass, no errors
  - Owner: Dev
  - Effort: 15 min

- [ ] **Regress-402**: Run linter and formatter
  - Commands: `golangci-lint run ./...`, `go fmt ./...`
  - Acceptance: No lint errors, code formatted
  - Owner: Dev
  - Effort: 10 min

- [ ] **Regress-403**: Code coverage report
  - Command: `go test -cover -coverprofile=coverage.out ./...`
  - Target: ≥85% for concurrent_resources.go
  - Owner: Dev
  - Effort: 10 min

### Commit & Documentation
- [ ] **Commit-401**: Commit Phase 4 tests
  - Message: "test(concurrent): Add comprehensive unit, integration, and E2E tests"
  - Files: custom_resources_test.go, integration_test.go, benchmark_test.go, e2e_concurrent_test.sh
  - Owner: Dev
  - Effort: 10 min

- [ ] **Commit-402**: Commit code coverage
  - Message: "test(concurrent): Verify code coverage ≥85%"
  - Files: coverage report (if tracked)
  - Owner: Dev
  - Effort: 5 min

---

## Documentation & Release Preparation

### Documentation Updates
- [ ] **Doc-201**: Update README.md with concurrency section
  - File: `ibcq-source-k8s/README.md`
  - Content:
    - Concurrency feature description
    - Configuration example
    - Performance expectations (5-10x faster)
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Section is clear and helpful

- [ ] **Doc-202**: Update docs/_configuration.md
  - File: `docs/_configuration.md`
  - Content: Concurrency config options, tuning guide
  - Owner: Dev
  - Effort: 30 min
  - Acceptance: Users understand concurrency settings

- [ ] **Doc-203**: Update CHANGELOG.md
  - File: `CHANGELOG.md`
  - Content: v0.5.0 release notes, features, improvements, breaking changes (none)
  - Owner: Dev
  - Effort: 20 min
  - Acceptance: Release notes are clear

- [ ] **Doc-204**: Create ARCHITECTURE.md section
  - File: `specs/003-concurrent-gvk/ARCHITECTURE.md` (new)
  - Content: Concurrent fetcher design, data flow, sequence diagram
  - Owner: Dev (Optional)
  - Effort: 30 min

### PR & Review
- [ ] **PR-001**: Create pull request on GitHub
  - Title: "feat: Implement concurrent GVK processing (Phase 1-4)"
  - Branch: `003-concurrent-gvk-processing` → `main` or `dev`
  - Description: Links to spec, lists validation results
  - Owner: Dev
  - Effort: 10 min

- [ ] **PR-002**: Ensure CI/CD passes
  - Action: Wait for GitHub Actions to complete
  - Check: All tests pass, linter passes, build succeeds
  - Owner: CI/CD
  - Effort: 10 min (wait)

- [ ] **PR-003**: Code review
  - Action: Request review from team
  - Approval: ≥1 approval required
  - Owner: Team Lead
  - Effort: 30 min (estimate)

- [ ] **PR-004**: Address review comments
  - Action: Make requested changes
  - Owner: Dev
  - Effort: Variable

- [ ] **PR-005**: Merge PR
  - Action: Squash and merge commits
  - Owner: Team Lead
  - Effort: 5 min

---

## Success Criteria Checklist

### Functionality
- [ ] All 4 phases implemented (Core, Resilience, Rate Limiting, Testing)
- [ ] Concurrent fetch uses errgroup with bounded concurrency
- [ ] One GVK failure doesn't block others (partial success)
- [ ] Per-GVK error collection and logging
- [ ] Exponential backoff for transient errors
- [ ] All 19 existing tests pass (no regression)
- [ ] 15+ new tests pass (concurrent behavior)

### Performance
- [ ] Single GVK time unchanged (±5%, no regression)
- [ ] 10-GVK sync time ≤ 1.5x slowest single GVK
- [ ] Concurrent speedup ≥3x (benchmark verified)
- [ ] Memory overhead ≤20% (benchmark verified)
- [ ] No goroutine leaks (confirmed by tests)

### Observability
- [ ] Logs show per-GVK duration, status, resource count
- [ ] Error messages are clear and actionable
- [ ] Partial failure summary shows GVK breakdown
- [ ] Benchmark results documented
- [ ] Code comments explain concurrent behavior

### Quality
- [ ] Code coverage ≥85% for concurrent_resources.go
- [ ] No lint errors (golangci-lint)
- [ ] Code formatted (gofmt)
- [ ] All comments spell-checked
- [ ] Documentation accurate and up-to-date

### Release Readiness
- [ ] CHANGELOG.md updated with v0.5.0
- [ ] README.md has concurrency section
- [ ] Configuration docs updated
- [ ] No breaking changes
- [ ] PR approved and merged
- [ ] Git tags updated (v0.5.0)

---

## Effort Summary

| Phase | Setup | Coding | Testing | Docs | Total |
|-------|-------|--------|---------|------|-------|
| 1 | 1 hr | 2 hrs | 1.5 hrs | 0.5 hr | 5 hrs |
| 2 | - | 1 hr | 1.5 hrs | 0.25 hr | 2.75 hrs |
| 3 | - | 1.5 hrs | 1.5 hrs | 0.25 hr | 3.25 hrs |
| 4 | - | 1 hr | 2.5 hrs | 0.5 hr | 4 hrs |
| Release | - | - | - | 1.5 hrs | 1.5 hrs |
| **TOTAL** | **1 hr** | **5.5 hrs** | **7 hrs** | **3 hrs** | **16.5 hrs** |

**Realistic Timeline**: 2-3 days (assuming 6-8 hrs/day development)

---

## Notes

- All commands assume macOS zsh shell
- Replace `./...` with specific package if needed for faster tests
- Benchmark comparisons use same hardware for accuracy
- Integration tests require real K8s cluster (minikube acceptable)
- E2E tests require PostgreSQL running
- Code review feedback may require additional iterations

---

## Quick Links

- **Specification**: `specs/003-concurrent-gvk/spec.md`
- **Implementation Plan**: `specs/003-concurrent-gvk/plan.md`
- **Code Location**: `resources/services/customresources/`
- **Tests Location**: `resources/services/customresources/*_test.go`
- **Issue**: (Link to GitHub issue if created)
- **PR**: (Link to PR once created)

---

**Status**: 🟡 Ready for Phase 1 Implementation  
**Last Updated**: 2026-03-17  
**Owner**: Dev Team  
