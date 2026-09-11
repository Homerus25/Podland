---
title: Fix Duplicate Connection Cache Declarations
date: 2026-07-02
status: completed
labels:
  - completed
  - bug
  - connections
author: vibe-agent
---

## What was built

Resolved duplicate connection cache type declarations and fixed Podman connection handling.

## Acceptance criteria

- [x] Removed duplicate ConnectionCache type declarations from manager.go
- [x] Removed duplicate ConnectionEntry type declarations from manager.go
- [x] Removed duplicate NewConnectionCache function from manager.go
- [x] Removed duplicate cleanup methods from manager.go
- [x] Fixed Podman connection creation using correct bindings API
- [x] Updated ConnectionEntry to properly store connection context and client
- [x] Ensured connection management works correctly with Podman bindings

## Blocked by

None

## Implementation details

### Changes made:

1. **Removed duplicates from manager.go**:
   - Removed duplicate `ConnectionCache` struct declaration
   - Removed duplicate `ConnectionEntry` struct declaration  
   - Removed duplicate `NewConnectionCache()` function
   - Removed duplicate `cleanupLoop()` and `cleanup()` methods

2. **Fixed Podman connection creation**:
   - Corrected usage of `bindings.NewConnectionWithOptions()` (returns context.Context)
   - Added proper usage of `bindings.GetClient()` to get actual connection
   - Updated `ConnectionEntry` struct to store both context and connection

3. **Fixed connection cleanup**:
   - Removed invalid `conn.Close()` calls (bindings.Connection doesn't have Close method)
   - Added proper comments explaining connection lifecycle management

### Before (broken):
```go
// Duplicate declarations in manager.go
type ConnectionCache struct { ... }
type ConnectionEntry struct { ... }

// Wrong Podman connection approach
conn, err := bindings.NewConnectionWithOptions(ctx, options)
entry.conn.Close() // Invalid - no Close method
```

### After (fixed):
```go
// Single declarations in cache.go only
type ConnectionCache struct { ... }
type ConnectionEntry struct {
    connCtx   context.Context  // Connection context
    conn      *bindings.Connection // Actual connection
    ...
}

// Correct Podman connection approach
connCtx, err := bindings.NewConnectionWithOptions(ctx, options)
conn, err := bindings.GetClient(connCtx)
// No Close() calls - managed automatically
```

## Verification

✅ **Duplicate declarations resolved** - All connection types now declared only in cache.go
✅ **Podman bindings compatibility** - Uses correct connection creation pattern
✅ **Connection lifecycle management** - Proper context + client handling
✅ **Build errors resolved** - No more connection cache related build failures

## Related issues

This fix resolves the connection cache portion of the master build issue `.scratch/000-fix-build-issues/master-issue.md`