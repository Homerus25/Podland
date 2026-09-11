--- 
title: Frontend API Integration for Machine Deletion
parent: implement machine deletion
date: 2026-07-03
status: ready-for-agent
labels:
  - machines
  - frontend
  - deletion
  - api
  - react-query
author: vibe-agent
---

## Parent

Part of the "implement machine deletion" feature.

## What to build

Implement the API integration layer for machine deletion using React Query mutations. This includes error handling, loading states, and automatic cache invalidation.

## Acceptance criteria

- [ ] React Query mutation hook created for DELETE `/api/machines/{id}`
- [ ] Mutation handles JWT authentication via Authorization: Bearer header
- [ ] Error responses properly categorized (401, 403, 404, 500)
- [ ] Loading states managed correctly
- [ ] Machine list cache invalidated after successful deletion
- [ ] Current machine detail cache invalidated if on detail page
- [ ] Redirect to machine list after deletion from detail view
- [ ] Error boundaries display appropriate messages

## Blocked by

- `/home/homerus/Projects/Podland/.scratch/020-machine-deletion-01-backend-service-delete/020-1-backend-service-delete-method.md` (Slice 1) - backend service must be complete

## Technical notes

- Create `useDeleteMachine` hook in `frontend/src/features/machines/hooks/`
- Use `useMutation` from React Query
- Endpoint: `DELETE /api/machines/{machineId}`
- Include machineId in mutation key for cache invalidation
- On success: invalidate `machines` query cache and optionally redirect
- On error: display context-appropriate error message
- Use axios or fetch with Authorization: Bearer header for JWT
- Return mutation object with `mutate`, `isLoading`, `isError`, `error`
- Follow existing hook patterns in the codebase