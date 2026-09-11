# Issue: Test ConnectionManager Concurrency and Timeout Handling

**Status**: ready-for-agent
**Created**: 2026-07-28
**Labels**: testing, connections, concurrency

## What to build

Implement comprehensive tests for ConnectionManager concurrency control and timeout handling. These tests should verify proper behavior under load and with timing constraints.

## Acceptance criteria

- [ ] Test concurrent connection requests are properly queued and processed in order
- [ ] Test queue full condition is handled correctly (returns appropriate error)
- [ ] Test timeout behavior when queue processing takes longer than queueTimeout
- [ ] Test context cancellation is properly handled in QueueRequestContext
- [ ] Test maximum concurrent connections limit is enforced (maxConcurrent parameter)
- [ ] Test behavior when multiple goroutines access the manager simultaneously
- [ ] Test cleanup of abandoned connection requests (context cancellation)
- [ ] Test behavior when connection processing is slower than request rate
- [ ] Test that timeouts don't leak goroutines or resources

## Blocked by

None - can start immediately

## Implementation notes

- Use Go's race detector to test concurrent access
- Create load tests with multiple concurrent requesters
- Test various timeout scenarios
- Focus on resource management under stress
- Use context.WithTimeout for testing timeout behavior
- Consider adding these to the same `manager_test.go` file or create `manager_concurrency_test.go`