---
title: Create Test Infrastructure and Mock Utilities
date: 2026-07-02
status: completed
labels:
  - needs-triage
  - testing
  - infrastructure
author: vibe-agent
---

## What to build

Create shared test infrastructure including mock interfaces, test helpers, and common test patterns for backend unit testing.

## Acceptance criteria

- [x] Create `internal/testutils/` package with shared testing helpers
- [x] Implement mock interfaces for repositories and external services
- [x] Create test data generators for common entities (users, machines, etc.)
- [x] Set up test configuration and environment management
- [x] Add helper functions for common test assertions
- [x] Document testing patterns and conventions

## Blocked by

None - can start immediately

## Technical details

### Components to implement:

1. **Mock Interfaces**:
   - `MockUserRepository` implementing `UserRepositoryInterface`
   - `MockMachineRepository` implementing `MachineRepositoryInterface`
   - `MockEncryptionService` for testing encryption/decryption
   - `MockConnectionCache` for testing connection management

2. **Test Helpers**:
   - `testutils.NewTestAuthService()` - Helper to create test auth service
   - `testutils.NewTestMachineService()` - Helper to create test machine service
   - `testutils.CreateTestJWTKeys()` - Helper for JWT test keys
   - `testutils.CleanupTestResources()` - Resource cleanup helper

3. **Test Data Generators**:
   - `testutils.GenerateTestUser()` - Create test user data
   - `testutils.GenerateTestMachine()` - Create test machine data
   - `testutils.GenerateValidPEMKey()` - Create valid test PEM keys
   - `testutils.GenerateInvalidPEMKey()` - Create invalid test PEM keys

4. **Test Configuration**:
   - Centralized test configuration management
   - Environment variable handling for tests
   - Test timeout and retry configurations

### Example Usage:

```go
// In individual test files
func TestMachineService(t *testing.T) {
    // Setup
    mockRepo := testutils.NewMockMachineRepository()
    encryption := testutils.NewMockEncryptionService()
    service := testutils.NewTestMachineService(mockRepo, encryption)
    
    // Test logic
    machine, err := service.CreateMachine(context.Background(), &models.Machine{...})
    
    // Assertions
    assert.NoError(t, err)
    assert.NotNil(t, machine)
    
    // Cleanup
    defer testutils.CleanupTestResources()
}
```

## Implementation approach

1. **Create testutils package**: `mkdir -p internal/testutils`
2. **Implement mock interfaces** using Go interfaces and struct embedding
3. **Add helper functions** for common test setup patterns
4. **Document usage** with examples in package documentation
5. **Ensure thread-safety** for shared test resources

## Success criteria

- All subsequent test slices can use this infrastructure
- Test setup code is DRY (Don't Repeat Yourself)
- Mocks are realistic and cover edge cases
- Test execution is fast and reliable