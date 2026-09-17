# Issue tracker: Forgejo

Issues and PRDs for this repo live as Forgejo issues. Use the forgejo MCP tools (`mcp_forgejo_*`) for all operations.

## Conventions

- **Create an issue**: `create_issue`. Bodies follow the house shape: `## Parent`, `## What to build`, `## Acceptance criteria`, `## Blocked by` (see existing issues for the established voice).
- **Read an issue**: `get_issue_by_index` + `list_issue_comments`.
- **List issues**: `list_repo_issues` / `search_issues` (for metadata-only looks keep the limit small — issue bodies are large).
- **Comment / close**: `create_issue_comment` / `issue_state_change`.
- **Apply / remove labels**: `add_issue_labels` / `remove_issue_labels` with the strings from `triage-labels.md`.
- **Blocking edges**: native — `add_issue_dependency` / `list_issue_dependencies`.

Infer owner and repo from `git remote -v` (origin is the Forgejo instance).

Forgejo shares one number space across issues and PRs, so a bare `#42` may be either — the response's `pull_request` field distinguishes.

## Pull requests as a triage surface

**PRs as a request surface: no.** Single-contributor repo; PRs are implementation slices, not incoming requests.

## When a skill says "publish to the issue tracker"

Create a Forgejo issue (in dependency order, blockers first, when publishing a set; declare blocking edges with `add_issue_dependency`). Apply the `ready-for-agent` label per `triage-labels.md`.

## When a skill says "fetch the relevant ticket"

Run `get_issue_by_index` (+ `list_issue_comments`).

## History

`.scratch/` is the retired local-markdown tracker (epics 000–460; from ~430 on issues were copied to Forgejo). Conventions docs still cite it as precedent. Do not file new issues there.
