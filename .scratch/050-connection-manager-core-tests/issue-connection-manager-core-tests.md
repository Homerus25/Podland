# Issue: Test ConnectionManager Core Functionality

**Status**: ready-for-agent
**Created**: 2026-07-28
**Labels**: testing, connections, unit-tests

## What to build

Implement comprehensive unit tests for the ConnectionManager core operations. These tests should verify the basic functionality of connection request queuing and processing.

## Acceptance criteria

- [ ] Test `NewConnectionManager` creates a properly initialized manager with correct dependencies
- [ ] Test `GetConnection` returns cached connections when available (cache hit)
- [ ] Test `GetConnection` queues requests when cache is full or returns error
- [ ] Test `QueueRequestContext` properly queues connection requests and returns response channel
- [ ] Test `processQueue` processes queued requests correctly and sends responses
- [ ] Test `Run` starts the connection manager and begins processing queue
- [ ] Test `Release` delegates to cache correctly
- [ ] Test basic queueing behavior with single request
- [ ] Test that successful connections are properly cached for reuse

## Blocked by

None - can start immediately

## Implementation notes

- Use testutils mocks for ConnectionCache, MachineRepository, and EncryptionService
- Test the interaction between ConnectionManager and ConnectionCache
- Focus on the core request/response flow
- Use context timeouts to test asynchronous behavior
- Create separate test file: `manager_test.go`