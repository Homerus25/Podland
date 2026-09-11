---
title: "Prefactor: extract MachineCard from the machines overview page"
parent: 180-machine-status-indicator
date: 2026-08-29
status: ready-for-agent
labels:
  - ready-for-agent
  - machines
  - frontend
  - prefactor
author: vibe-agent
---

## Parent

`.scratch/180-machine-status-indicator/issue.md`

## What to build

The machines overview page currently defines its machine card as an inline component inside the page. Extract the card into the machines feature's shared components, next to the existing permission badge, so the upcoming status indicator lands in a self-contained, independently testable component.

This is a pure structural move: same props, same rendering, same behavior, same test ids. No visual or functional change of any kind. The existing overview page tests are the safety net — they must pass unmodified.

## Acceptance criteria

- [ ] Machine card renders exactly as before: name, description, host:port, permission badge, View button
- [ ] The overview page consumes the extracted card component from the machines feature
- [ ] The card's existing test id is preserved
- [ ] All existing machines overview page tests pass without modification
- [ ] No leftover card markup in the page component

## Blocked by

None - can start immediately
