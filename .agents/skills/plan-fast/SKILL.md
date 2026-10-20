---
name: plan-fast
description: Plan a new feature end-to-end without the interview — the agent grills the plan itself, adopts its own best answers, then writes the spec and tickets.
disable-model-invocation: true
user-invocable: true
argument-hint: "What feature do you want to plan?"
---

# Plan Fast

Turn a feature idea into a Spec-Issue and tickets on the tracker, deepening the plan yourself instead of interviewing the user.

The issue tracker and triage label vocabulary should have been provided to you. If not, tell the user to run `/setup-matt-pocock-skills`.

## Process

### 1. Self-grill

Run the grilling process against yourself. Build the **design tree**: every decision branches into the decisions that hang off it. Work the **frontier** in **rounds** — each round, take every decision whose prerequisites are settled, and instead of asking the user:

- Write the question and the realistic options.
- Give your recommended answer, and name at least one alternative you rejected and why. An answer with no rejected alternative was not thought about.
- Adopt the answer and move on.

Finding facts is your job: explore the codebase or dispatch a sub-agent for anything a question depends on. If two options are genuinely tied, you are missing a fact — go find it.

**Done when:** the frontier is empty — every branch of the design tree visited, every decision adopted with a one-line rationale, nothing silently assumed.

### 2. Report

Present the adopted decisions as a compact list: each decision, the answer, the rejected alternative. The decisions are already adopted — the report is informational, so publish it and continue immediately.

**Done when:** the decision list is in front of the user.

### 3. Assess size

Judge whether the feature is **small** (one tracer bullet, one fresh context window) or **large** (multiple tickets across layers).

**Done when:** you can state in one sentence why the feature is small or large.

### 4. Small — one ticket

Call the Skill tool with `to-tickets`. Publish a single ticket, labeled `ready-for-agent`.

**Done when:** one ticket exists on the tracker, labeled `ready-for-agent`.

### 5. Large — spec, tickets, wire

**a. Spec.** Call the Skill tool with `to-spec`. It publishes a Spec-Issue, labeled `ready-for-agent`. Note the issue number.

**b. Tickets.** Call the Skill tool with `to-tickets`. Split the tickets along the backend/frontend seam — each ticket is backend-only or frontend-only. Publish all tickets, labeled `ready-for-agent`.

**c. Wire.** Set every ticket as a blocker of the Spec-Issue, using the tracker's native dependency mechanism. The Spec-Issue is "Blocked by" all tickets.

**Done when:** the Spec-Issue exists, all tickets exist and are labeled `ready-for-agent`, every ticket blocks the Spec-Issue, and the backend/frontend split is visible in ticket titles or descriptions.

## No-interview mode

`to-spec` and `to-tickets` both contain check-with-the-user steps. Skip them: adopt your recommended answer, record it in the artifact (spec decision, ticket description), and continue.
