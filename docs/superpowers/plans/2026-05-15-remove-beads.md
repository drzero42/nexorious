# Remove Beads Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fully remove the beads issue-tracking integration from this repository, leaving no traces in config, files, git history refs, or documentation.

**Architecture:** beads is integrated via five surfaces: (1) `bd setup claude` managed sections in `CLAUDE.md` and `.claude/settings.json`, (2) `bd setup codex` managed section in `AGENTS.md`, (3) project-local skills in `.claude/skills/`, (4) the `.beads/` directory (committed data + local runtime files), (5) remote Dolt refs on origin. Each surface has its own removal method. The `bd setup` commands must run before `.beads/` is deleted, because they need the beads binary to be functional.

**Tech Stack:** `bd` CLI (beads), git, standard shell tools

**Prerequisites:** All open beads issues are closed. No export needed.

---

### Task 1: Remove bd Claude Code and Codex integrations

These two `bd setup --remove` commands handle the managed content automatically. Run them while the `.beads/` database still exists.

**Files:**
- Modify: `.claude/settings.json` (SessionStart hook removed)
- Modify: `CLAUDE.md` (lines 221–313 removed — entire managed block containing `## Task Tracking` and `## Session Completion`)
- Modify: `AGENTS.md` (lines 50–end removed — the `<!-- BEGIN/END BEADS INTEGRATION -->` block)

- [ ] **Step 1: Remove Claude Code integration**

```bash
bd setup claude --remove
```

Expected output: confirmation that `.claude/settings.json` hook and `CLAUDE.md` section were removed.

- [ ] **Step 2: Verify CLAUDE.md managed block is gone**

```bash
grep -n "BEGIN BEADS\|END BEADS\|bd prime\|Task Tracking\|Session Completion" CLAUDE.md
```

Expected output: no matches.

- [ ] **Step 3: Verify settings.json hook is gone**

```bash
cat .claude/settings.json
```

Expected: no `SessionStart` or `bd prime` entries. The file should contain only `enabledPlugins` and `permissions` keys.

- [ ] **Step 4: Remove Codex/AGENTS.md integration**

```bash
bd setup codex --remove
```

Expected output: confirmation that `AGENTS.md` managed section was removed.

- [ ] **Step 5: Verify AGENTS.md managed block is gone**

```bash
grep -n "BEGIN BEADS\|END BEADS\|bd prime\|Beads Issue Tracker" AGENTS.md
```

Expected output: no matches.

---

### Task 2: Stop Dolt server and delete project-local beads skills

**Files:**
- Delete: `.claude/skills/brainstorming/SKILL.md`
- Delete: `.claude/skills/epic-executor/SKILL.md`
- Delete: `.claude/skills/next-task/SKILL.md`
- Delete: `.claude/skills/plan-to-epic/SKILL.md`

- [ ] **Step 1: Stop the embedded Dolt server**

```bash
bd dolt stop 2>/dev/null || true
```

Expected: no errors (or silent success if server was not running).

- [ ] **Step 2: Delete the four project-local beads skills**

```bash
rm -rf .claude/skills/brainstorming \
       .claude/skills/epic-executor \
       .claude/skills/next-task \
       .claude/skills/plan-to-epic
```

- [ ] **Step 3: Verify skills are gone**

```bash
ls .claude/skills/ 2>/dev/null && echo "directory still has contents" || echo "directory empty or gone"
```

Expected: directory is empty or does not exist. If it still exists with other contents, that is fine — only the four listed directories must be gone.

---

### Task 3: Untrack .beads/ from git and delete it locally

The `.beads/` directory has 11 committed files that must be removed from the git index, plus untracked runtime files (ignored by `.beads/.gitignore`) that must be deleted from disk.

**Files:**
- Delete (tracked): `.beads/.gitignore`, `.beads/README.md`, `.beads/config.yaml`, `.beads/metadata.json`, `.beads/interactions.jsonl`, `.beads/issues.jsonl`, `.beads/hooks/post-checkout`, `.beads/hooks/post-merge`, `.beads/hooks/pre-commit`, `.beads/hooks/pre-push`, `.beads/hooks/prepare-commit-msg`
- Delete (untracked runtime): `.beads/embeddeddolt/`, `.beads/backup/`, `.beads/export-state.json`, `.beads/last-touched`, `.beads/.local_version`

- [ ] **Step 1: Untrack all .beads/ files from git**

```bash
git rm -r .beads/
```

Expected output: `rm '.beads/.gitignore'`, `rm '.beads/README.md'`, etc. — one line per tracked file.

- [ ] **Step 2: Delete the remaining .beads/ directory and all runtime files**

```bash
rm -rf .beads/
```

- [ ] **Step 3: Verify .beads/ is fully gone**

```bash
ls .beads/ 2>/dev/null && echo "ERROR: directory still exists" || echo "OK: .beads/ is gone"
```

Expected: `OK: .beads/ is gone`

---

### Task 4: Clean git config and .gitignore

**Files:**
- Modify: `.git/config` (unset `beads.role`)
- Modify: `.gitignore` (remove 5-line beads block)

- [ ] **Step 1: Unset beads.role from git config**

```bash
git config --unset beads.role
```

Expected: no output (silent success).

- [ ] **Step 2: Verify git config is clean**

```bash
git config --get beads.role 2>/dev/null && echo "ERROR: still set" || echo "OK: not set"
```

Expected: `OK: not set`

- [ ] **Step 3: Remove beads entries from .gitignore**

Open `.gitignore` and remove this block (it appears after the `.env` line):

```
# Beads / Dolt files (added by bd init)
.dolt/
*.db
.beads-credential-key
.beads/proxieddb/
```

The surrounding content to use as anchors:
- The line before the block is: `.env`
- The line after the block is: (blank line, then `# Test coverage files`)

After removal, `.env` should be followed directly by a blank line and `# Test coverage files`.

- [ ] **Step 4: Verify .gitignore is clean**

```bash
grep -n "beads\|\.dolt\|proxieddb" .gitignore
```

Expected output: no matches.

---

### Task 5: Move Non-Interactive Shell Commands to CLAUDE.md, delete AGENTS.md

After Task 1, `AGENTS.md` contains only the original hand-written content (lines 1–49 of the original file): a beads preamble (lines 1–24), a beads Quick Reference (lines 16–24), and the Non-Interactive Shell Commands section (lines 26–49). The preamble and quick reference are beads-only and are deleted. The Non-Interactive Shell Commands section is useful and is moved to `CLAUDE.md`.

**Files:**
- Modify: `CLAUDE.md` (add Non-Interactive Shell Commands section before `## Known Gotchas`)
- Delete: `AGENTS.md`

- [ ] **Step 1: Add Non-Interactive Shell Commands to CLAUDE.md**

In `CLAUDE.md`, find `## Known Gotchas` and insert the following block immediately before it (keep a blank line between the preceding section and this new section):

```markdown
### Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var
```

This section belongs under `## Development Rules` — insert it after the `### Slumber Collection Maintenance` subsection and before `## Known Gotchas`.

- [ ] **Step 2: Verify Non-Interactive Shell Commands section is in CLAUDE.md**

```bash
grep -n "Non-Interactive Shell Commands" CLAUDE.md
```

Expected: one match, located between `### Slumber Collection Maintenance` and `## Known Gotchas`.

- [ ] **Step 3: Delete AGENTS.md**

```bash
rm -f AGENTS.md
```

- [ ] **Step 4: Verify AGENTS.md is gone**

```bash
ls AGENTS.md 2>/dev/null && echo "ERROR: still exists" || echo "OK: deleted"
```

Expected: `OK: deleted`

- [ ] **Step 5: Final CLAUDE.md sanity check — no beads references remain**

```bash
grep -in "beads\|bd prime\|bd dolt\|bd ready\|bd create\|bd close\|bd update\|bd show\|plan-to-epic\|epic-executor\|next-task\|brainstorming.*pipeline\|superpowers:brainstorming" CLAUDE.md
```

Expected output: no matches.

---

### Task 6: Commit all changes

- [ ] **Step 1: Stage all changes**

```bash
git add -A
```

- [ ] **Step 2: Verify what is staged**

```bash
git status
```

Expected staged changes:
- Deleted: `.beads/.gitignore`, `.beads/README.md`, `.beads/config.yaml`, `.beads/hooks/*` (5 files), `.beads/interactions.jsonl`, `.beads/issues.jsonl`, `.beads/metadata.json`
- Deleted: `AGENTS.md`
- Modified: `.claude/settings.json`
- Modified: `CLAUDE.md`
- Modified: `.gitignore`
- Deleted: `.claude/skills/brainstorming/SKILL.md`, `.claude/skills/epic-executor/SKILL.md`, `.claude/skills/next-task/SKILL.md`, `.claude/skills/plan-to-epic/SKILL.md`

- [ ] **Step 3: Commit**

```bash
git commit -m "chore: remove beads issue tracker integration"
```

---

### Task 7: Remove remote Dolt refs and push

beads stored its Dolt database under two non-standard refs on the origin remote: `refs/dolt/data` (the database) and `refs/heads/__dolt_remote_info__` (Dolt metadata). These must be deleted explicitly.

- [ ] **Step 1: Delete __dolt_remote_info__ branch from origin**

```bash
git push origin --delete __dolt_remote_info__
```

Expected: `- [deleted] __dolt_remote_info__`

- [ ] **Step 2: Delete refs/dolt/data from origin**

```bash
git push origin :refs/dolt/data
```

Expected: `- [deleted] refs/dolt/data`

- [ ] **Step 3: Push the removal commit**

```bash
git push
```

- [ ] **Step 4: Final verification — no Dolt refs remain on origin**

```bash
git ls-remote origin | grep -i dolt
```

Expected output: no matches.

- [ ] **Step 5: Final verification — local state is clean**

```bash
git status
```

Expected: `nothing to commit, working tree clean` and `up to date with 'origin/...'`
