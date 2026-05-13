---
description: Find the next unblocked beads task and work on it. Use when the user wants to continue work, pick up where they left off, or asks what to do next.
invocation: explicit
---

# Next Task

Find the next unblocked task in beads and execute it.

## Step 1: Check for in-progress work

```bash
bd list --status in_progress --json
```

If any tasks are in progress, resume the most recently updated one rather than starting something new. Show the user what you found and confirm before continuing.

## Step 2: Find the next ready task

If nothing is in progress:

```bash
bd ready --json
```

If no tasks are ready, run:

```bash
bd blocked --json
```

Report what is blocked and why, then stop — do not proceed until the user resolves the blockers.

## Step 3: Identify the epic

From the ready task's `parent` field, check epic context:

```bash
bd show <epic-id> --json
```

This gives you the goal and architecture context to orient the work.

## Step 4: Claim and execute

Claim the task:

```bash
bd update <task-id> --claim --json
```

Then dispatch a fresh subagent with:
- The full task description (`bd show <task-id>` Description field)
- The design context (Design field)
- The epic goal for architectural orientation
- Instructions to ask clarifying questions before starting if anything is unclear

## Step 5: Review

After the subagent completes:

1. **Spec compliance** — verify the implementation matches the task description line by line. Do not trust the implementer's self-report; read the actual code.
2. **Code quality** — use `superpowers:code-reviewer` once spec compliance passes.

If either review finds issues, dispatch a fix subagent with the specific findings, then re-review.

## Step 6: Close and sync

```bash
bd close <task-id> --reason "Implemented and verified" --json
bd dolt push
```

Report what was completed and suggest running `/next-task` again to continue.