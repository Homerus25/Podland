# Issue: Test ConnectionCache Core Functionality

**Status**: completed
**Created**: 2026-07-28
**Labels**: testing, connections, unit-tests

## What to build

Implement comprehensive unit tests for the ConnectionCache core operations in the `@backend/internal/connections/` package. The tests should verify the basic functionality of connection caching, creation, and management.

## Acceptance criteria

- [x] Test `NewConnectionCache` creates a properly initialized cache with correct maxSize
- [x] Test `GetOrCreate` returns existing connections when available (cache hit)
- [x] Test `GetOrCreate` creates new connections when needed (cache miss)
- [x] Test `GetOrCreateContext` works correctly and returns connection contexts
- [x] Test `Release` properly decrements reference counts and marks connections as idle when refCount reaches 0
- [x] Test `MarkActive` updates connection active status and lastUsed timestamp
- [x] Test `MarkIdle` updates connection idle status and lastUsed timestamp
- [x] Test `evictLRUIdle` removes the least recently used idle connection when cache is full
- [x] Test `cleanupLoop` runs periodically and calls cleanup
- [x] Test `cleanup` removes idle connections that haven't been used for the idle timeout period

## Blocked by

None - can start immediately

## Implementation notes

- Use the existing test patterns from the codebase (testify/assert, testutils mocks)
- Create test files following the naming convention: `cache_test.go`
- Use table-driven tests for comprehensive coverage
- Mock dependencies like MachineRepository and EncryptionService using testutils
- Test both happy paths and edge cases within the core functionality scope