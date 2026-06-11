---
name: work-on-issue
description: Use when the user asks to work on, pick up, implement, fix, or start a GitHub issue by number (e.g. "/work-on-issue 825", "work on issue 825", "let's do #825"). Primes superpowers, verifies the issue's claims instead of trusting them, routes between a direct fix and the full spec/plan workflow, and gets the user's approval before making any changes.
---

# Work on a GitHub Issue

## Overview

Turn a GitHub issue number into well-scoped, verified work. An issue is a **report and a request, not a specification** — its claims may be stale, wrong, or based on a misunderstanding of the code. Your job is to confirm what is actually true, decide how much process the work warrants, and surface anything ambiguous to the user before writing code.

The issue number is in the skill arguments (e.g. `825`). If no number was given, ask the user for one before continuing.

## Step 1 — Prime superpowers

**REQUIRED:** Invoke superpowers:using-superpowers first if it is not already active this session. It establishes how to find and apply the other skills you'll route into (brainstorming, writing-plans, executing-plans, TDD, debugging).

## Step 2 — Read the issue

```bash
gh issue view <number> --comments
```

Read the body **and** all comments — later comments often correct, narrow, or supersede the original report. Note the labels, linked PRs, and any referenced issues.

## Step 3 — Verify every claim (do NOT blindly follow)

Treat each factual or instructional statement in the issue as a hypothesis to confirm against the actual codebase — not a fact to act on.

- A claim like "X is broken because of Y in `file.go`" → open `file.go` and confirm Y is really the cause. Reporters frequently misdiagnose.
- A proposed fix or "just change A to B" → verify A exists, that B is correct, and that it doesn't break callers or other behavior.
- A described symptom → reproduce it if feasible (run the test, hit the endpoint, check the data) before assuming it's real and current.
- References to files/functions/flags → confirm they still exist; issues age and the code moves.

If a claim is **wrong or outdated**, stop and tell the user what you actually found rather than implementing against a false premise. The right fix is often not the one the issue proposes.

## Step 3.5 — Check for prior design docs and process precedent

List `docs/superpowers/specs/` and `docs/superpowers/plans/` (listing only — do **not** read docs for unrelated or completed work; landed code is the authority, and stale plans mislead).

- If a doc covers **this issue or its epic's still-open design**, read it — that design is settled; don't re-brainstorm it.
- For a child of an epic, the siblings' trail (spec+plan vs. plan-only vs. neither) is precedent for this child's lane. Follow it unless this child is materially different, and say so if you deviate.

## Step 4 — Decide whether to brainstorm

Use superpowers:brainstorming **with the user** when the issue involves design choices, unclear intent, multiple plausible approaches, user-facing behavior, or anything creative. Skip it for a mechanical, unambiguous fix where the correct change is obvious and verified.

When unsure whether brainstorming is warranted, lean toward a short brainstorming pass — it's cheaper than building the wrong thing.

## Step 5 — Route: direct fix vs. full spec/plan cycle

Judge the size and risk of the **verified** work (not the issue's framing) and pick a lane:

| Signal | Lane |
|---|---|
| Single-file or localized change, clear correct fix, low risk, no design decisions | **Direct approach** — branch, TDD the change, verify, PR |
| Touches multiple subsystems, schema/migration, new feature, ambiguous design, security-sensitive, or "large" by feel | **Full cycle** — superpowers:writing-plans (and a spec if design is open) → superpowers:executing-plans |

This is a judgment call, not a formula. If it's borderline, ask the user which they'd prefer.

Either lane still follows the project's mandatory workflow (feature branch, migrations as new files, tests, quality gates) from CLAUDE.md.

## Step 6 — Get the user's approval before making any changes

**Do not start implementing on your own initiative.** Once you've verified the issue and chosen a lane, present a short summary to the user and **wait for their explicit approval before you touch anything**:

- What you actually found when verifying (especially if it differs from the issue's claims).
- The lane you propose (direct fix vs. full cycle) and why.
- The concrete change you intend to make — the file(s), the approach, and roughly what "done" looks like.

Then **stop and let the user respond.** "Making changes" includes creating the branch, writing the first test, and editing code — none of it begins until the user says go. Announcing your plan is not the same as getting approval; you must actually pause for their answer.

This gate holds even when the fix is obvious and verified. A one-line typo fix still gets a one-line "here's what I'll do — okay to proceed?" The point is the user, not you, decides when implementation starts.

If the user has already told you in this session to proceed without checking in (e.g. "just fix it, don't ask"), honor that — their explicit instruction overrides this gate.

## Step 7 — Ask on any uncertainty

Whenever the issue is ambiguous, claims conflict, scope is unclear, or you're unsure what "done" means — **ask the user** rather than guessing. A clarifying question now is cheaper than a wrong implementation. Use AskUserQuestion for concrete either/or decisions.

## Red flags — STOP

- "The issue says to change X, so I'll change X" — without having read X yourself. **Verify first.**
- "This looks simple, I'll skip brainstorming" — on something with design or behavior choices. **Brainstorm.**
- "I'll just start coding" — before deciding the lane or branching. **Route first.**
- "I verified it and it's obviously right, so I'll go ahead and implement" — **no.** Present your plan and wait for the user's approval first (Step 6). Verified ≠ approved.
- "I stated my lane, so I can proceed" — announcing is not approval. **Stop and wait for their answer.**
- "I'll just branch and write the test while I'm here" — branching and the first test ARE making changes. **Not until approved.**
- "I'll assume they mean…" — on an ambiguous point. **Ask.**
- Implementing the issue's proposed fix after finding its diagnosis was wrong. **Tell the user what you found.**
