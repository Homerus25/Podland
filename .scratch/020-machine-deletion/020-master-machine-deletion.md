--- 
title: Implement Machine Deletion
parent: N/A
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - deletion
  - epic
  - high-priority
author: vibe-agent
---

## What to build

Complete machine deletion functionality across the full stack, including backend service logic, data cleanup, connection cache invalidation, frontend UI, API integration, and comprehensive tests.

## Child Issues ( implementations in dependency order )

### Backend Implementation (Start Here)
1. **020-1**: Backend Service Delete Method - `020-machine-deletion-01-backend-service-delete/020-1-backend-service-delete-method.md`
2. **020-2**: Machine Access Cleanup - `020-machine-deletion-02-access-cleanup/020-2-machine-access-cleanup.md`
3. **020-3**: Connection Cache Invalidation - `020-machine-deletion-03-connection-cache/020-3-connection-cache-invalidation.md`

### Frontend Implementation (Can Start in Parallel with Backend)
4. **020-4**: Frontend Delete Button and Dialog - `020-machine-deletion-04-frontend-delete-ui/020-4-frontend-delete-button-and-dialog.md`
5. **020-5**: Frontend API Integration - `020-machine-deletion-05-frontend-api/020-5-frontend-api-integration.md`

### Testing (After Implementation)
6. **020-6**: Backend Unit Tests - `020-machine-deletion-06-backend-tests/020-6-backend-unit-tests-deletion.md`
7. **020-7**: Frontend Component Tests - `020-machine-deletion-07-frontend-tests/020-7-frontend-component-tests-deletion.md`

## Dependencies

```
020-1 (Backend Service)
    ↓
020-2 (Access Cleanup) → 020-6 (Backend Tests)
    ↓
020-3 (Cache Invalidation) ↗
    ↓
020-6 (Backend Tests)

020-4 (Frontend UI) → 020-7 (Frontend Tests)
    ↓
020-5 (Frontend API) → 020-7 (Frontend Tests)
```

## Success Criteria

- [ ] Machines can be deleted by their owners
- [ ] All related data (access entries, connections) properly cleaned up
- [ ] Frontend provides confirmation before deletion
- [ ] Appropriate error handling at all levels
- [ ] All tests pass
- [ ] No memory or database leaks

## Current State

The backend already has:
- DELETE endpoint in handler (`handler.go:201-242`)
- Repository Delete method (`repository.go:114-124`) 
- Route wired up (`main.go:132`)

Missing:
- Service layer orchestration
- Data cleanup (access entries, connection cache)
- Frontend UI and API integration
- Comprehensive tests

## Recommendation

Start with **020-1** (Backend Service Delete Method) as it's the foundation, then proceed through the backend slices (020-2, 020-3), then frontend (020-4, 020-5), and finally tests (020-6, 020-7).

Frontend UI (020-4) can be worked on in parallel with backend implementation.