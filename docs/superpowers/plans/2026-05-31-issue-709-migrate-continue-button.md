# Issue #709 — Migration Continue Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After a successful migration run, show a Continue button so the user can read the log before being redirected to `/`.

**Architecture:** Single function `showSuccess()` added to `ui/migrate/index.html`, called from the `complete` SSE event handler in place of the immediate `checkStatusAndAct()` call. The Continue button's click handler takes over the status-check-and-redirect logic that previously ran automatically.

**Tech Stack:** Vanilla JS, HTML — no build step required.

---

### Task 1: Create feature branch

- [ ] **Step 1: Create and switch to a feature branch**

```bash
git checkout -b fix/issue-709-migrate-continue-button
```

Expected: switched to a new branch `fix/issue-709-migrate-continue-button`

---

### Task 2: Add `showSuccess()` and update the `complete` SSE handler

**Files:**
- Modify: `ui/migrate/index.html:72-125`

- [ ] **Step 1: Add `showSuccess()` after the existing `showFailure()` function**

In `ui/migrate/index.html`, locate `showFailure` (currently ends around line 79). Add the new function immediately after it:

```js
    function showSuccess() {
      var btn = document.getElementById('btn');
      var status = document.getElementById('status');
      status.textContent = 'Migrations complete — click Continue to proceed';
      status.className = 'meta meta--success';
      btn.textContent = 'Continue';
      btn.onclick = function() {
        btn.disabled = true;
        checkStatusAndAct()
          .then(function() { startPolling(); })
          .catch(function() {
            status.textContent = 'Could not verify migration status. Refresh to retry.';
            status.className = 'meta meta--error';
            btn.disabled = false;
          });
      };
      btn.disabled = false;
    }
```

- [ ] **Step 2: Update the `complete` SSE event handler**

In `ui/migrate/index.html`, find the `complete` event listener (currently lines 110–125):

```js
          es.addEventListener('complete', function() {
            completed = true;
            es.close();
            // RunMigrations closes the SSE channel before the handler runs
            // InitNeedsSetup and calls TransitionToReady, so the server may
            // still report state="migrating" briefly. checkStatusAndAct
            // handles ready/migration_failed; the resumed poll catches up
            // for the brief still-migrating window.
            checkStatusAndAct()
              .then(function() { startPolling(); })
              .catch(function() {
                status.textContent = 'Could not verify migration status. Refresh to retry.';
                status.className = 'meta meta--error';
                btn.disabled = false;
              });
          });
```

Replace it with:

```js
          es.addEventListener('complete', function() {
            completed = true;
            es.close();
            showSuccess();
          });
```

- [ ] **Step 3: Commit**

```bash
git add ui/migrate/index.html
git commit -m "fix: show Continue button after migration instead of auto-redirecting"
```

---

### Task 3: Verify the change

The migration UI is a server-rendered HTML page. Verify manually:

- [ ] **Step 1: Build and run the server**

```bash
make build
export DATABASE_URL="<your-dev-db-url>"
export DB_ENCRYPTION_KEY="<your-dev-key>"
./nexorious
```

- [ ] **Step 2: Visit `/migrate` and run migrations**

Open `http://localhost:8000/migrate` in a browser. Click **Run Migrations**. Observe that:
1. The log fills with migration output as before.
2. After the last log line, the status line shows **"Migrations complete — click Continue to proceed"** in green (`meta--success` colour).
3. The button is re-enabled and reads **"Continue"**.
4. The page does **not** redirect automatically.

- [ ] **Step 3: Click Continue and verify redirect**

Click **Continue**. Observe that:
1. The button disables immediately.
2. The page redirects to `/` once the server confirms `state=ready`.

- [ ] **Step 4: Verify the failure path is unchanged**

If you can trigger a migration failure (e.g., break the DB connection mid-run), confirm the existing failure behaviour is unaffected: button re-enables as "Retry", status shows the error in red.

---

### Task 4: Open PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin fix/issue-709-migrate-continue-button
```

- [ ] **Step 2: Create the PR**

```bash
gh pr create \
  --title "fix: show Continue button after migration instead of auto-redirecting" \
  --body "$(cat <<'EOF'
Closes #709.

After a successful migration run the page now shows a **Continue** button instead of redirecting immediately. The log output stays visible until the user clicks Continue.

## Changes

- `ui/migrate/index.html`: new `showSuccess()` function; `complete` SSE handler calls it instead of `checkStatusAndAct()`. The status-check-and-redirect logic moves into the Continue click handler, preserving the existing polling fallback for the brief window where the server may still report `state=migrating`.

## Testing

Manually verified:
- [ ] Migration success: Continue button appears, log stays visible, redirect fires on click
- [ ] Migration failure: existing Retry flow unchanged
EOF
)"
```
