# Next.js Remnants Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all Next.js artefacts from the Vite + React frontend in a single pass.

**Architecture:** Pure dead-code and stale-comment removal — no logic changes anywhere. Four categories: package.json entries, `'use client'` directives, dead test mocks, stale path-header comments.

**Tech Stack:** Vite 7, React 19, TypeScript, TanStack Router, Vitest

**Spec:** `docs/superpowers/specs/2026-05-03-nextjs-cleanup-design.md`

---

### Task 1: Create feature branch

**Files:** none

- [ ] **Step 1: Create and check out branch**

```bash
git checkout -b fix/nextjs-cleanup
```

Expected: `Switched to a new branch 'fix/nextjs-cleanup'`

---

### Task 2: Update package.json and reinstall

**Files:**
- Modify: `frontend/package.json`

- [ ] **Step 1: Remove `next`, `eslint-config-next`, and rename package**

Open `frontend/package.json`. Make three edits:

1. Change `"name"` from `"frontend-next"` to `"frontend"`
2. Remove the `"next": "^16.0.10"` line from `dependencies`
3. Remove the `"eslint-config-next": "^16.0.10"` line from `dependencies`

The `dependencies` block should no longer contain either `next` or `eslint-config-next`.

- [ ] **Step 2: Reinstall to regenerate lockfile**

```bash
cd frontend && npm install
```

Expected: `added N packages` or similar — no errors.

- [ ] **Step 3: Verify ESLint still works**

```bash
npm run check
```

Expected: zero errors. (The ESLint config `eslint.config.mjs` does not reference `eslint-config-next`, so removing the package has no ESLint impact.)

- [ ] **Step 4: Commit**

```bash
cd ..
git add frontend/package.json frontend/package-lock.json
git commit -m "chore: remove next and eslint-config-next packages, rename package to frontend"
```

---

### Task 3: Strip `'use client'` directives

**Files (39 total):**
- Modify: all `.tsx`/`.ts` files in `frontend/src/` that contain `use client`

The `'use client'` directive is a Next.js React Server Components instruction. It is a no-op in a Vite SPA and should not be present.

- [ ] **Step 1: Remove directive lines with sed**

```bash
cd frontend
grep -rl "use client" src --include="*.tsx" --include="*.ts" | \
  xargs sed -i -e "/^'use client';$/d" -e '/^"use client"$/d'
```

This handles both quote styles (`'use client';` used in custom files and `"use client"` used in shadcn/ui generated files).

- [ ] **Step 2: Verify no occurrences remain**

```bash
grep -r "use client" src --include="*.tsx" --include="*.ts"
```

Expected: no output.

- [ ] **Step 3: Type-check and lint**

```bash
npm run check
```

Expected: zero errors.

- [ ] **Step 4: Run tests**

```bash
npm run test
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
cd ..
git add frontend/src
git commit -m "chore: remove 'use client' directives (Next.js no-ops in Vite)"
```

---

### Task 4: Remove dead test mocks

**Files:**
- Modify: `frontend/src/components/games/game-card.test.tsx`
- Modify: `frontend/src/components/games/game-list.test.tsx`
- Modify: `frontend/src/components/navigation/nav-section.test.tsx`

The production components no longer import from `next/image` or `next/navigation`. These mocks are dead code.

- [ ] **Step 1: Remove the `next/image` mock from `game-card.test.tsx`**

The file starts with these six imports followed by the mock block. Remove lines 8–29 (the comment and entire `vi.mock('next/image', ...)` block including its trailing blank line):

```
// Mock next/image - filter out Next.js specific props that aren't valid HTML attributes
vi.mock('next/image', () => ({
  default: ({ src, alt, fill, unoptimized, priority, sizes, ...props }: {
    src: string;
    alt: string;
    fill?: boolean;
    unoptimized?: boolean;
    priority?: boolean;
    sizes?: string;
    [key: string]: unknown
  }) => (
    <img
      src={src}
      alt={alt}
      data-fill={fill ? "true" : undefined}
      data-unoptimized={unoptimized ? "true" : undefined}
      data-priority={priority ? "true" : undefined}
      data-sizes={sizes}
      {...props}
    />
  ),
}));
```

After removal the first non-import line should be the `// Mock the env config` comment.

- [ ] **Step 2: Remove the identical `next/image` mock from `game-list.test.tsx`**

Same block — lines 8–29, same content as above. After removal the first non-import line should be `// Mock the env config`.

- [ ] **Step 3: Remove the `next/navigation` mock from `nav-section.test.tsx`**

Remove lines 8–12 (comment + mock block + trailing blank line):

```
// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: vi.fn(() => '/other'),
  useRouter: vi.fn(() => ({ push: vi.fn() })),
}));
```

After removal the next line should be the `describe('NavSectionCollapsible', () => {` block.

- [ ] **Step 4: Run tests to confirm nothing broke**

```bash
cd frontend && npm run test
```

Expected: all tests pass. The mocks were dead code — removing them changes nothing about test behaviour.

- [ ] **Step 5: Commit**

```bash
cd ..
git add frontend/src/components/games/game-card.test.tsx \
        frontend/src/components/games/game-list.test.tsx \
        frontend/src/components/navigation/nav-section.test.tsx
git commit -m "chore: remove dead next/image and next/navigation test mocks"
```

---

### Task 5: Remove stale path-header comments

**Files (14 total):**
- `frontend/src/api/sync.test.ts` — comment reads `// frontend-next/src/api/sync.test.ts`
- `frontend/src/components/navigation/index.ts`
- `frontend/src/components/navigation/nav-items.tsx`
- `frontend/src/components/navigation/nav-link.test.tsx`
- `frontend/src/components/navigation/nav-section.test.tsx`
- `frontend/src/components/navigation/nav-section.tsx`
- `frontend/src/components/navigation/types.ts`
- `frontend/src/components/ui/nav-badge.test.tsx`
- `frontend/src/components/ui/nav-badge.tsx`
- `frontend/src/lib/env.ts`
- `frontend/src/main.tsx`
- `frontend/src/routes/__root.tsx`
- `frontend/src/routes/_authenticated.tsx`
- `frontend/vite.config.ts`

These first-line comments contain old file paths copied from when the project was in a `frontend-next/` or `frontend/src/` directory structure. They are stale and misleading.

- [ ] **Step 1: Remove the `// frontend-next/` comment from `sync.test.ts`**

```bash
cd frontend
sed -i '/^\/\/ frontend-next\//d' src/api/sync.test.ts
```

- [ ] **Step 2: Remove all `// frontend/` header comments in `src/`**

```bash
grep -rl "^// frontend/" src --include="*.tsx" --include="*.ts" | \
  xargs sed -i '/^\/\/ frontend\//d'
```

- [ ] **Step 3: Remove the path comment from `vite.config.ts`**

```bash
sed -i '/^\/\/ frontend\//d' vite.config.ts
```

- [ ] **Step 4: Verify no stale comments remain**

```bash
grep -r "^// frontend" src --include="*.tsx" --include="*.ts"
grep "^// frontend" vite.config.ts
```

Expected: no output from either command.

- [ ] **Step 5: Run full verification**

```bash
npm run check
npm run test
```

Expected: zero errors, all tests pass.

- [ ] **Step 6: Commit**

```bash
cd ..
git add frontend/src frontend/vite.config.ts
git commit -m "chore: remove stale Next.js path-header comments"
```

---

### Task 6: Open PR

- [ ] **Step 1: Push branch**

```bash
git push -u origin fix/nextjs-cleanup
```

- [ ] **Step 2: Open PR**

```bash
gh pr create \
  --title "chore: remove Next.js remnants from frontend" \
  --body "$(cat <<'EOF'
## Summary
- Removes `next` and `eslint-config-next` packages from `package.json`
- Renames package from `frontend-next` to `frontend`
- Strips 39 `'use client'` directives (Next.js no-ops in Vite)
- Removes dead `vi.mock('next/image')` and `vi.mock('next/navigation')` mocks from 3 test files
- Removes stale path-header comments from 14 files

No logic changes. All tests pass.

## Test plan
- [ ] `npm run check` passes (zero tsc + eslint errors)
- [ ] `npm run test` passes (all tests)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
