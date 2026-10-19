---
name: plan-feature
description: Plan a new feature end-to-end — grill the idea, write a spec, break it into backend and frontend tickets.
disable-model-invocation: true
user-invocable: true
argument-hint: "What feature do you want to plan?"
---

# Plan Feature

Turn a feature idea into a grilled Spec-Issue and a set of tickets on the issue tracker.

The issue tracker and triage label vocabulary should have been provided to you. If not, tell the user to run `/setup-matt-pocock-skills`.

## Process

### 1. Grill

Call the Skill tool with `grill-me`. Work the session to completion: the frontier is empty, every branch of the design tree visited, nothing left silently assumed.

**Done when:** the user confirms a shared understanding.

### 2. Assess size

Judge whether the grilled feature is **small** (one tracer bullet, one fresh context window) or **large** (multiple tickets across layers).

**Done when:** you can state in one sentence why the feature is small or large.

### 3. Small — one ticket

Call the Skill tool with `to-tickets`. Publish a single ticket, labeled `ready-for-agent`.

**Done when:** one ticket exists on the tracker, labeled `ready-for-agent`.

### 4. Large — spec, tickets, wire

**a. Spec.** Call the Skill tool with `to-spec`. It publishes a Spec-Issue, labeled `ready-for-agent`. Note the issue number.

**b. Tickets.** Call the Skill tool with `to-tickets`. Split the tickets along the backend/frontend seam — each ticket is backend-only or frontend-only. Publish all tickets, labeled `ready-for-agent`.

**c. Wire.** Set every ticket as a blocker of the Spec-Issue, using the tracker's native dependency mechanism. The Spec-Issue is "Blocked by" all tickets.

**Done when:** the Spec-Issue exists, all tickets exist and are labeled `ready-for-agent`, every ticket blocks the Spec-Issue, and the backend/frontend split is visible in ticket titles or descriptions.
