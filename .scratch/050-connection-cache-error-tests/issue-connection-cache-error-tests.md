# Issue: Test ConnectionCache Error Handling and Edge Cases

**Status**: ready-for-agent
**Created**: 2026-07-28
**Labels**: testing, connections, error-handling

## What to build

Implement comprehensive error handling and edge case tests for ConnectionCache. These tests should verify robust error handling and boundary conditions.

## Acceptance criteria

- [ ] Test error handling when machine repository fails (GetByID returns error)
- [ ] Test error handling when encryption service fails (Decrypt returns error)
- [ ] Test error handling when SSH key decryption fails with invalid data
- [ ] Test error handling when Podman connection creation fails
- [ ] Test behavior when cache is full and all connections are active (no idle connections to evict)
- [ ] Test concurrent access to the cache (thread safety with multiple goroutines)
- [ ] Test cleanup behavior with zero idle timeout
- [ ] Test release behavior with negative reference counts
- [ ] Test behavior when trying to release non-existent connections
- [ ] Test behavior when machine is not found in repository

## Blocked by

None - can start immediately

## Implementation notes

- Use testutils mock repositories with error injection capabilities
- Test race conditions using Go's race detector
- Create edge case scenarios that might not be covered in happy path testing
- Focus on error propagation and proper error messages
- Test resource cleanup in error scenarios (e.g., temp files are removed even when connection creation fails)