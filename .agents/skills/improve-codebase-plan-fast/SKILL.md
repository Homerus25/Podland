---
name: improve-codebase-plan-fast
description: Scan a codebase for deepening opportunities, present them as a visual HTML report, then run plan-fast on every strong candidate via subagents and wire one session ticket blocked by all generated tickets.
disable-model-invocation: true
user-invocable: true
---

# Improve Codebase Plan Fast

Surface architectural friction and propose **deepening opportunities**: refactors that turn shallow modules into deep ones. The aim is testability and AI-navigability. Where `improve-codebase-architecture` grills the user through one picked candidate, this skill plans every **Strong** candidate automatically and publishes the results to the issue tracker.

## Process

### 1-2. Explore and report

Run steps 1 (Explore) and 2 (Present candidates as an HTML report) of the `improve-codebase-architecture` skill exactly as written: read [`../improve-codebase-architecture/SKILL.md`](../improve-codebase-architecture/SKILL.md) and follow its Explore and HTML-report steps, including its [`HTML-REPORT.md`](../improve-codebase-architecture/HTML-REPORT.md). Keep its "do not propose interfaces yet" rule — the subagents design; the report only names the deepening.

One difference: the report is informational. Skip its closing question ("Which of these would you like to explore?") — after writing and opening the file, tell the user the absolute path and continue immediately. Every candidate keeps its `Strong` / `Worth exploring` / `Speculative` badge: the badge drives the next step.

**Done when:** the HTML report is written, opened, and its absolute path is in front of the user, with every candidate badged.

### 3. Dispatch plan-fast per Strong candidate

For every candidate badged **Strong**, spawn one subagent. Its first message invokes `/plan-fast` with the candidate: the candidate's title, files, problem, and solution from the report, plus the report's absolute path so the subagent can read the full card. Spawn all of them up front and let them run in parallel; each adopts its own decisions (plan-fast's no-interview mode) and publishes its spec and tickets to the configured tracker.

Collect from each subagent's result every ticket number it created.

**Done when:** every Strong candidate has a finished plan-fast run behind it, and you hold every ticket number created this session.

### 4. Session ticket

Create one ticket on the configured tracker that records the session: the report's absolute path, the Strong candidates planned, and a link to each candidate's spec. Wire it as blocked by every ticket created this session — each candidate's Spec-Issue and its tickets — using the tracker's native dependency mechanism. With no Strong candidates, the ticket records the review with an empty blocker list.

**Done when:** the session ticket exists on the tracker and every ticket created this session is wired as its blocker.
