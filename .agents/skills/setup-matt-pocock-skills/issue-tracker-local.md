# Issue tracker: Local Markdown

Issues for this repo live as markdown files in `.scratch/`.

## Conventions

- One feature per directory family, prefixed with a three-digit epic number: `.scratch/<NNN>-<feature-slug>/`
- The epic issue is `.scratch/<NNN>-<feature-slug>/issue.md` (problem statement, solution, slice list)
- Implementation slices are one directory each, numbered from `01` in dependency order (blockers first): `.scratch/<NNN>-<feature-slug>-<NN>-<slice-slug>/issue.md` — `00` marks a prefactor slice
- Every issue file starts with YAML frontmatter: `title`, `parent`, `date` (YYYY-MM-DD), `status`, `labels`, `author`
- Triage state is the frontmatter `status:` value (see `triage-labels.md` for the role strings)
- A slice body has: `## Parent`, `## What to build`, `## Acceptance criteria`, `## Blocked by` (referencing blocking slice numbers/titles)
- Comments and conversation history append to the bottom of the file under a `## Comments` heading

## When a skill says "publish to the issue tracker"

Create new files under `.scratch/` following the layout above (creating directories if needed).

## When a skill says "fetch the relevant ticket"

Read the file at the referenced path. The user will normally pass the path or the issue number directly.
