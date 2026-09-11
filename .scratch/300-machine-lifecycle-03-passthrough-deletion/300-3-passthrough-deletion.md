---
title: Pass-through deletion
parent: machine lifecycle module - owner rules behind one interface, deletion with full teardown
date: 2026-09-09
status: ready-for-agent
labels:
  - ready-for-agent
  - backend
  - machines
  - refactoring
author: vibe-agent
---

## Parent

Machine lifecycle epic (300-0-machine-lifecycle-master) - read its "Design
decisions" section first; decision 5 governs this slice.

## What to build

Three machine-service methods are pass-throughs with zero production
callers, and one is a byte-identical duplicate of another: the
name-uniqueness check (the create path already uses the repository
directly), the duplicate of the accessible-machines list, and the
owned-machines list. They fail the deletion test: delete them and nothing
reappears elsewhere. The machines list endpoint keeps working unchanged by
riding the one real list method.

## Acceptance criteria

- [ ] The three zero-caller service pass-throughs (name-uniqueness wrapper, duplicate accessible-list, owned-list) are deleted
- [ ] The machines list endpoint's behaviour is unchanged: same response, same permission carried per machine
- [ ] No production code references the deleted methods
- [ ] Tests that only asserted the pass-throughs are deleted; tests with real behavioural value are re-pointed at the surviving service method

## Blocked by

- 300-2-sharing-rules-extraction
