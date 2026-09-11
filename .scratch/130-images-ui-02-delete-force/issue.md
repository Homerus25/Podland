---
title: Images UI — delete with confirmation and force path for in-use images
parent: 130-images-ui
date: 2026-08-21
status: open
labels:
  - ready-for-agent
  - frontend
  - images
  - feature
author: vibe-agent
---

## Parent

`.scratch/130-images-ui/issue.md`

## What to build

Add a delete action to each image row in the Images tab. Deleting requires a confirmation dialog so accidents are impossible. When the backend rejects the delete with `IMAGE_USED_BY_CONTAINERS`, the modal explains that containers use the image and offers an explicit force-delete option, so the user understands the failure and can choose force deliberately. Successful delete (normal or force) invalidates the images query so the list refreshes; other errors surface through the shared error display.

## Acceptance criteria

- [ ] Delete button on each image row opens a confirmation modal; nothing is deleted without explicit confirm
- [ ] On `IMAGE_USED_BY_CONTAINERS`, the modal explains the in-use situation and offers a force-delete choice
- [ ] Successful delete or force-delete invalidates the images query so the row disappears without manual reload
- [ ] Non-in-use errors render via the shared error envelope display
- [ ] Vitest + RTL tests with MSW: delete confirmation flow, in-use error → force path, query invalidation

## Blocked by

- `.scratch/130-images-ui-01-image-list/issue.md`
