---
name: epic-executor
description: Execute a beads epic using subagent-driven development - sequential task execution with two-stage review (spec compliance, then code quality)
---

# Epic Executor

Execute all tasks within a beads epic using subagent-driven development. Tasks run sequentially with two-stage review after each.

## Usage

```
/epic-executor <epic-id>
```

## Setup Phase

### 1. Validate Epic

```bash
bd show <epic-id> --json
bd epic status <epic-id> --json
```

If epic doesn't exist or is already 100% complete, inform user and stop.

### 2. Note Base SHA

Record git SHA before starting for code review context:

```bash
git rev-parse HEAD
```

## Execution Loop

Process one task at a time until epic complete:

### 1. Check Completion

```bash
bd epic status <epic-id> --json
```

If 100% complete, run `bd dolt push` to sync, then announce completion and stop.

### 2. Get Next Ready Task

```bash
bd ready --parent=<epic-id> --limit=1 --json
```

If no tasks ready but epic incomplete, check `bd blocked --parent=<epic-id> --json` and report what's blocking.

### 3. Claim Task

```bash
bd update <task-id> --claim --json
```

### 4. Dispatch Implementer Subagent

Use subagent-driven-development pattern. Dispatch a fresh subagent with:
- Full task description from `bd show <task-id>` (the Description field)
- Task design context (the Design field)
- Epic context
- Instructions to ask questions before starting
- Self-review checklist before reporting back

**If subagent asks questions**: Answer them, then let them continue.

### 5. Spec Compliance Review

Dispatch spec reviewer subagent to verify implementation matches specification.

**Critical:** The reviewer must read actual code and compare to requirements line by line. It does NOT trust the implementer's self-report.

Report: APPROVED or NEEDS_CHANGES with specific file:line references.

### 6. Code Quality Review

**Only after spec compliance passes.**

Use `superpowers:code-reviewer` subagent. Focus on:
- Security issues
- Test coverage
- Best practices for the project's language/framework
- Performance concerns

### 7. Handle Review Results

**If either review finds issues:**
1. Dispatch fix subagent with specific issues to address
2. Re-run the review that found issues
3. Repeat until both reviews pass

**If both reviews pass:** Proceed to close.

### 8. Close Task

```bash
bd close <task-id> --reason="Implemented and verified" --json
```

### 9. Continue

Go back to step 1 for next task.

## Completion

When `bd epic status <epic-id> --json` shows 100%:

```bash
bd dolt push
```

```
Epic <epic-id> complete!

Summary:
- Tasks completed: N
- All implementations reviewed and verified
```

## Key Principles

1. **Sequential execution** - One task at a time, respecting dependencies
2. **Fresh subagent per task** - No context pollution between tasks
3. **Two-stage review** - Spec compliance first, then code quality
4. **Fix before closing** - Tasks with review issues get fixed, not skipped
5. **Use beads, not TodoWrite** - Track state with `bd update` and `bd close`
6. **Always use --json** - All `bd` commands use `--json` for reliable parsing
7. **Push on completion** - Always run `bd dolt push` when the epic finishes