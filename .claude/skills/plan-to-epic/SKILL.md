---
name: plan-to-epic
description: Convert a superpowers implementation plan into a beads epic with properly structured tasks, acceptance criteria, and inferred dependencies
---

# Plan to Epic

Convert implementation plans (from `superpowers:writing-plans`) and design documents (from `superpowers:brainstorming`) into beads epics ready for `epic-executor`.

## Usage

```
/plan-to-epic <path-to-plan.md>
/plan-to-epic docs/plans/2026-01-01-feature-plan.md
/plan-to-epic docs/plans/2026-01-01-feature-plan.md --design docs/plans/2026-01-01-feature-design.md
/plan-to-epic docs/plans/2026-01-01-feature-plan.md --epic-id platform-xyz
```

**Flags:**
- `--design <path>` - Include design document for additional context (architecture, data flow, decisions)
- `--epic-id <id>` - Add tasks to existing epic instead of creating new one

## Workflow

### Step 1: Read and Parse Documents

**Read the plan file and extract:**

From header:
- Title and goal
- Architecture overview
- Tech stack

From each task section:
- Task title
- Files to modify
- Step-by-step instructions
- Code snippets
- Expected outcomes

**If `--design` provided, also extract:**

- Architecture decisions and rationale
- Data flow descriptions
- Error handling strategies
- Component relationships
- Testing approach
- Any constraints or requirements

Store this as `design_context` for inclusion in tasks.

### Step 2: Create Epic

If `--epic-id` not provided, create new epic:

```bash
bd create --type=epic --priority=0 --json \
  --title="<Plan Title>" \
  --body-file=/tmp/epic-body.md
```

### Step 3: Analyze Dependencies

**File overlap detection:**
- Track which files each task modifies
- If Task N modifies a file that Task M also modifies, and M > N, then Task M depends on Task N

**Explicit references:**
- Scan task content for patterns like "after Task N", "builds on Task N"
- Create explicit dependency

### Step 4: Build Task Content

For each task, create content for three separate fields:

**Description (--body-file):** Implementation-focused content - what to do:
- Task summary
- Files to modify
- Implementation steps (full detail, not summarized)
- Code snippets
- Testing requirements
- Verification steps
- Commit guidance

**Design (--design):** Context and rationale - why and how it fits:
- Epic goal and architecture context
- Tech stack from plan header
- Relevant design decisions (extracted from design doc if provided)

**Notes (--notes):** Source document references as fallback:
- Plan path with line numbers for this task
- Design path if provided

### Step 5: Extract Acceptance Criteria

Build from multiple sources:
- "Expected:" lines in the plan
- Verification steps
- Test descriptions
- Standard criteria (tests pass, linter passes, code committed)

### Step 6: Create Tasks

```bash
bd create --type=task --priority=0 --json \
  --parent=<epic-id> \
  --title="<Task Title>" \
  --acceptance="<acceptance criteria>" \
  --body-file=/tmp/task-N-body.md \
  --design "$(cat /tmp/task-N-design.md)" \
  --notes "Source: Plan <path> (lines N-M); Design <path>"
```

### Step 7: Add Dependencies

```bash
bd dep add <dependent-task-id> <dependency-task-id> --json
```

### Step 8: Output Summary

```
Created epic: platform-abc "Feature Name"

Source documents:
  Plan: docs/plans/2026-01-01-feature-plan.md
  Design: docs/plans/2026-01-01-feature-design.md

Tasks (9):
  platform-abc.1: Task 1 Title [no dependencies]
    - Description: 847 words (implementation)
    - Design: 156 words (context)
  platform-abc.2: Task 2 Title [depends on: .1]
  ...

Ready to execute:
  /epic-executor platform-abc
```

## Key Principles

1. **Separation of concerns** - Description for implementation, Design for context, Notes for references
2. **Preserve full detail** - Never summarize implementation steps
3. **Comprehensive acceptance criteria** - Extract from all sources
4. **Always use --json** - All `bd` commands use `--json` for reliable parsing
5. **Include design context** - When available, add relevant design decisions in the Design field