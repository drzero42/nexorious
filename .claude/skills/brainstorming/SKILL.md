---
name: brainstorming
description: "You MUST use this before any creative work - creating features, building components, adding functionality, or modifying behavior. Explores user intent, requirements and design, then chains into full implementation pipeline."
---

# Brainstorming Ideas Into Designs

## Overview

Help turn ideas into fully formed designs and specs through natural collaborative dialogue, then execute them to completion.

Start by understanding the current project context, then ask questions one at a time to refine the idea. Once you understand what you're building, present the design in small sections (200-300 words), checking after each section whether it looks right so far.

**This skill chains into the full implementation pipeline:**
```
Brainstorming -> Design Doc -> Implementation Plan -> Beads Epic -> Execution
```

## The Process

**Understanding the idea:**
- Check out the current project state first (files, docs, recent commits)
- Ask questions one at a time to refine the idea
- Prefer multiple choice questions when possible, but open-ended is fine too
- Only one question per message - if a topic needs more exploration, break it into multiple questions
- Focus on understanding: purpose, constraints, success criteria

**Exploring approaches:**
- Propose 2-3 different approaches with trade-offs
- Present options conversationally with your recommendation and reasoning
- Lead with your recommended option and explain why

**Presenting the design:**
- Once you believe you understand what you're building, present the design
- Break it into sections of 200-300 words
- Ask after each section whether it looks right so far
- Cover: architecture, components, data flow, error handling, testing
- Be ready to go back and clarify if something doesn't make sense

## After the Design

### Phase 1: Documentation

- Write the validated design to `docs/plans/YYYY-MM-DD-<topic>-design.md`
- Commit the design document to git

### Phase 2: Implementation Plan

Ask: **"Design complete. Ready to create the implementation plan?"**

If yes:
- Use `superpowers:writing-plans` to create detailed implementation plan
- Output: `docs/plans/YYYY-MM-DD-<topic>-plan.md`
- Commit the plan to git

### Phase 3: Plan Review (CHECKPOINT)

Present the plan summary to user:

```
Implementation plan created: docs/plans/YYYY-MM-DD-<topic>-plan.md

Tasks: <N>
Estimated complexity: <low/medium/high based on task count and dependencies>

Review the plan and confirm:
- Are the tasks correctly scoped?
- Are there any missing steps?
- Ready to create the beads epic?
```

**Wait for user approval before proceeding.**

If user requests changes:
- Update the plan
- Re-present for approval

### Phase 4: Epic Creation

Once plan is approved:
- Use `/plan-to-epic docs/plans/YYYY-MM-DD-<topic>-plan.md`
- Creates beads epic with:
  - All tasks as P0 priority
  - Acceptance criteria from "Expected:" lines
  - Dependencies inferred from file overlap
- Output: Epic ID (e.g., `platform-xyz`)

### Phase 5: Execution

Ask: **"Epic created: <epic-id>. Ready to start execution?"**

If yes:
- Use `/epic-executor <epic-id>`
- Sequential subagent execution with dependency awareness
- Two-stage review (spec compliance + code quality)
- Loops until epic 100% complete

## Key Principles

- **One question at a time** - Don't overwhelm with multiple questions
- **Multiple choice preferred** - Easier to answer than open-ended when possible
- **YAGNI ruthlessly** - Remove unnecessary features from all designs
- **Explore alternatives** - Always propose 2-3 approaches before settling
- **Incremental validation** - Present design in sections, validate each
- **Be flexible** - Go back and clarify when something doesn't make sense
- **Plan is the checkpoint** - User reviews plan before epic creation
- **Execution is autonomous** - Once approved, epic-executor runs to completion

## When NOT to Use Full Pipeline

For very small changes (single file, few lines), skip the pipeline:
- Quick bug fixes
- Minor refactors
- Documentation updates
- Config changes

Ask: "This seems small enough to do directly. Should I just implement it, or go through the full planning process?"