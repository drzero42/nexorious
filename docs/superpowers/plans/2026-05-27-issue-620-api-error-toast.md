# API Error Toast Detail (Issue #620 item 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make backend `{"error": "..."}` response bodies surface as the toast message instead of the generic `HTTP <status>: <statusText>` fallback.

**Architecture:** Frontend-only, additive. Add an `error` branch to the single error-extraction function `handleApiError` in `ui/frontend/src/api/client.ts`. All request paths (`apiCall`, `apiUploadFile`, `apiDownloadFile`) funnel through it, so one edit fixes every path. No backend changes.

**Tech Stack:** React 19 + TypeScript, Vitest + MSW (`msw`) for the frontend test.

**Spec:** `docs/superpowers/specs/2026-05-27-issue-620-api-error-toast-design.md`

---

### Task 1: Surface the backend `error` key in `handleApiError`

**Files:**
- Modify: `ui/frontend/src/api/client.ts:57-76` (the `handleApiError` function)
- Test: `ui/frontend/src/api/client.test.ts` (add to the existing `error handling` describe block, currently starting at line 361)

All commands run from `ui/frontend/`.

- [ ] **Step 1: Write the failing test**

In `ui/frontend/src/api/client.test.ts`, inside the `describe('error handling', () => { ... })` block, add this test immediately after the existing `it('extracts error message from message field', ...)` case (after line 398):

```ts
it('extracts error message from error field', async () => {
  server.use(
    http.get(`${API_URL}/error`, () => {
      return HttpResponse.json({ error: 'invalid or expired token' }, { status: 401 });
    }),
  );

  await expect(apiCall('/error')).rejects.toMatchObject({
    message: 'invalid or expired token',
    status: 401,
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm run test client.test.ts`

Expected: the new `extracts error message from error field` case FAILS. The pre-fix `handleApiError` reads only `detail` and `message`, so for an `{ error: ... }` body `errorMessage` stays at the `HTTP 401: ...` default — the `toMatchObject({ message: 'invalid or expired token' })` assertion does not match. All other `error handling` cases still pass.

- [ ] **Step 3: Add the `error` branch to `handleApiError`**

In `ui/frontend/src/api/client.ts`, change the body-parsing chain inside `handleApiError` (currently lines 65-69) from:

```ts
      if (typeof details.detail === 'string') {
        errorMessage = details.detail;
      } else if (typeof details.message === 'string') {
        errorMessage = details.message;
      }
```

to (insert the `error` branch between `detail` and `message`):

```ts
      if (typeof details.detail === 'string') {
        errorMessage = details.detail;
      } else if (typeof details.error === 'string') {
        errorMessage = details.error;
      } else if (typeof details.message === 'string') {
        errorMessage = details.message;
      }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npm run test client.test.ts`

Expected: ALL `client.ts` tests PASS, including the new `extracts error message from error field` case and the existing `detail` / `message` / non-JSON-fallback cases (the additive branch leaves their behavior unchanged).

- [ ] **Step 5: Type-check and lint**

Run: `npm run check`

Expected: no TypeScript or ESLint errors. (`npm run knip` is unaffected — no exports added or removed.)

- [ ] **Step 6: Commit**

```bash
git add ui/frontend/src/api/client.ts ui/frontend/src/api/client.test.ts
git commit -m "fix: surface backend error-key detail in API toasts (#620)"
```

---

### Task 2: Record the item-2 drop on issue #620

This is project housekeeping, not code. The spec drops item 2 (the refresh-retry regression test) because issue #625 removes the refresh mechanism it would test. Post a comment on #620 recording the decision.

> **Note:** Posting to GitHub is outward-facing. If the user prefers to post this themselves, hand them the text below instead of running the command.

- [ ] **Step 1: Post the decision comment**

```bash
gh issue comment 620 --body "Item 1 (toast loses backend error detail) addressed in branch \`fix/issue-620-api-error-toast\`: the frontend's \`handleApiError\` now reads the backend's standard \`error\` response key, fixing generic \`HTTP <status>\` toasts API-wide (not just auth 401s).

Item 2 (refresh-and-retry regression test) is intentionally **not** implemented here. It would guard \`refreshTokensFn\` and the refresh-on-401 retry in \`client.ts\`, but #625 (Replace JWT auth with server-side sessions and API keys) removes that mechanism entirely. A regression test for code already slated for deletion would be removed one milestone later, so it is dropped rather than carried."
```

Expected: the comment posts successfully and `gh` prints the comment URL.

---

## Notes for the implementer

- Precedence is `detail` → `error` → `message` → default. This is deliberate (see spec); no single backend body carries more than one of these keys, so the order is for readability/predictability, not behavior. Do **not** remove the `detail` or `message` branches — they cover the maintenance middleware and Echo's default error handler respectively.
- This branch is `fix/issue-620-api-error-toast`. The squash-merge PR **title** is what release-please parses, so keep it as a `fix:` Conventional Commit (patch bump). Reference `#620` in the PR body.
