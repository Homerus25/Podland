---
title: Master Issue - Implement Comprehensive Backend Testing
date: 2026-07-02
status: open
labels:
  - needs-triage
  - testing
  - master
  - quality
author: vibe-agent
---

## What to build

Implement comprehensive unit testing for the Podland backend to ensure code quality, prevent regressions, and enable safe refactoring.

## Acceptance criteria

- [ ] Test infrastructure and mock utilities implemented
- [ ] Auth service fully tested with >80% coverage
- [ ] Machine service fully tested with >85% coverage
- [ ] Connection management tested
- [ ] API handlers tested with mocked services
- [ ] Test coverage reporting configured
- [ ] CI integration complete
- [ ] All tests pass in CI pipeline
- [ ] Overall test coverage >75%

## Blocked by

None - can start immediately

## Sub-issues

### 1. Test Infrastructure (Blocker) - COMPLETED ✅
- **Issue**: `.scratch/010-test-infrastructure/test-issue.md`
- **Status**: Completed
- **Description**: Create shared test utilities, mock interfaces, and helpers
- **Dependencies**: None

### 2. Auth Service Tests - PENDING
- **Issue**: `.scratch/011-auth-service-tests/test-issue.md`
- **Status**: Not started
- **Description**: Comprehensive JWT token generation and validation tests
- **Dependencies**: Test infrastructure (Issue 1)
- **Coverage Target**: >80%

### 3. Machine Service Tests - PENDING
- **Issue**: `.scratch/012-machine-service-tests/test-issue.md`
- **Status**: Not started
- **Description**: Machine creation, validation, and PEM parsing tests
- **Dependencies**: Test infrastructure (Issue 1)
- **Coverage Target**: >85%

### 4. Connection Management Tests - PLANNED
- **Issue**: (To be created after infrastructure)
- **Status**: Not started
- **Description**: Connection caching and Podman connection tests
- **Dependencies**: Test infrastructure (Issue 1)
- **Coverage Target**: >80%

### 5. API Handler Tests - PLANNED
- **Issue**: (To be created after service tests)
- **Status**: Not started
- **Description**: HTTP handler tests with mocked services
- **Dependencies**: Auth, Machine, Connection tests (Issues 2-4)
- **Coverage Target**: >70%

### 6. CI Integration - PLANNED
- **Issue**: (To be created last)
- **Status**: Not started
- **Description**: Test coverage reporting and CI pipeline setup
- **Dependencies**: All other test issues (Issues 1-5)

## Progress Tracking

| Area | Status | Coverage Target | Current Coverage |
|------|--------|-----------------|------------------|
| Test Infrastructure | ✅ Completed | N/A | 100% |
| Auth Service | ❌ Not Started | >80% | 0% |
| Machine Service | ❌ Not Started | >85% | ~15% (existing) |
| Connection Management | ❌ Not Started | >80% | 0% |
| API Handlers | ❌ Not Started | >70% | 0% |
| CI Integration | ❌ Not Started | N/A | 0% |

**Overall Progress**: 1/6 issues completed (16.67%)

## Implementation Strategy

### Phase 1: Foundation (Week 1)
1. **Set up test infrastructure** - Create mocks, helpers, and utilities
2. **Implement auth service tests** - Critical security component
3. **Enhance machine service tests** - Build on existing tests

### Phase 2: Core Services (Week 2)
4. **Test connection management** - Complex stateful component
5. **Test remaining services** - Containers, images, quadlets

### Phase 3: Integration (Week 3)
6. **Test API handlers** - HTTP layer with mocked services
7. **Set up CI/CD integration** - Test reporting and gating

## Testing Principles

1. **Unit Test Focus**: Isolated tests with mocked dependencies
2. **Fast Execution**: All tests should run in <30 seconds
3. **Deterministic**: Tests should not depend on external state
4. **Comprehensive**: Cover happy paths, edge cases, and errors
5. **Maintainable**: Clear test names and organization
6. **Documentation**: Tests serve as living documentation

## Current State Analysis

### Existing Tests
- ✅ `internal/domain/machines/service_test.go` - Basic PEM validation tests
- ✅ `internal/domain/auth/service_test.go` - Skeleton with skipped tests
- ❌ No test infrastructure or mocks
- ❌ No CI integration
- ❌ Low overall test coverage

### Test Coverage Gaps
1. **No mock infrastructure** - Hard to test services in isolation
2. **No HTTP handler tests** - API layer untested
3. **No connection tests** - Complex stateful logic untested
4. **No CI integration** - Tests not running automatically
5. **Incomplete auth tests** - Critical security component untested

## Success Metrics

- **Code Coverage**: >75% overall, >80% for critical components
- **Test Reliability**: 0 flaky tests, 100% pass rate
- **Test Speed**: Full test suite <30 seconds
- **CI Integration**: Tests run on every PR, coverage reported
- **Developer Experience**: Easy to add new tests, good examples

## Next Steps

1. **Start with test infrastructure** - This unblocks all other testing
2. **Prioritize auth service** - Critical security component
3. **Enhance existing tests** - Build on machine service tests
4. **Add CI integration** - Ensure tests run automatically

## Resources Needed

- Test data and examples
- Mocking library (already have testify)
- CI/CD configuration access
- Time allocation for test writing

## Risk Assessment

- **Low Risk**: Test infrastructure and mocks
- **Medium Risk**: Complex service interactions
- **High Risk**: HTTP handler testing with authentication
- **Mitigation**: Start simple, build complexity gradually