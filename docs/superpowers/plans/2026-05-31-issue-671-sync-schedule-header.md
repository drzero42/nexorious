# Issue #671 — Move Sync Schedule into Header Card

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the sync frequency dropdown from the collapsible connection section into the Platform Header Card on the storefront detail page, stacked below the connection status badge.

**Architecture:** Single-file change in `$storefront.tsx`. The right column of the Platform Header Card gains a `flex-col` stack (badge on top, frequency Select below). The standalone "Sync Frequency" `Card` inside the `<Collapsible>` is deleted. All existing state (`effectiveFrequency`, `handleFrequencyChange`, `isUpdating`) is reused unchanged.

**Tech Stack:** React 19, TanStack Router, shadcn/ui Select, Tailwind CSS v4

---

### Task 1: Create feature branch

**Files:**
- No file changes

- [ ] **Step 1: Create and switch to the feature branch**

```bash
git checkout -b feat/issue-671-sync-schedule-header
```

- [ ] **Step 2: Verify you're on the right branch**

```bash
git branch --show-current
```

Expected output: `feat/issue-671-sync-schedule-header`

---

### Task 2: Move frequency dropdown into the Platform Header Card

**Files:**
- Modify: `ui/frontend/src/routes/_authenticated/sync/$storefront.tsx`

The right column of the Platform Header Card is the `<div className="ml-auto">` at line ~430. It currently holds only the badge. Change it to a vertical stack and add the frequency `Select` below the badge. Then delete the standalone "Sync Frequency" `Card` inside the `<Collapsible>`.

- [ ] **Step 1: Update the right column of the Platform Header Card**

Find this block (lines ~430–465 in `$storefront.tsx`):

```tsx
            <div className="ml-auto">
              <Badge
                variant={credentialsError ? 'destructive' : 'outline'}
                className={
                  credentialsError
                    ? 'cursor-pointer'
                    : config.isConfigured
                      ? 'cursor-pointer bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                      : 'cursor-pointer bg-muted text-muted-foreground'
                }
                onClick={() => setConnectionSectionOpen((o) => !o)}
              >
                {credentialsError ? (
                  <>
                    Credentials Error
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                ) : config.isConfigured ? (
                  <>
                    Connected
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                ) : (
                  <>
                    Not Configured
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                )}
              </Badge>
            </div>
```

Replace it with:

```tsx
            <div className="ml-auto flex flex-col items-end gap-2">
              <Badge
                variant={credentialsError ? 'destructive' : 'outline'}
                className={
                  credentialsError
                    ? 'cursor-pointer'
                    : config.isConfigured
                      ? 'cursor-pointer bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
                      : 'cursor-pointer bg-muted text-muted-foreground'
                }
                onClick={() => setConnectionSectionOpen((o) => !o)}
              >
                {credentialsError ? (
                  <>
                    Credentials Error
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                ) : config.isConfigured ? (
                  <>
                    Connected
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                ) : (
                  <>
                    Not Configured
                    <ChevronDown
                      className={`ml-1 h-3 w-3 transition-transform ${connectionSectionOpen ? 'rotate-180' : ''}`}
                    />
                  </>
                )}
              </Badge>
              {config.isConfigured && (
                <Select
                  value={effectiveFrequency}
                  onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
                  disabled={isUpdating}
                >
                  <SelectTrigger className="w-[140px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.values(SyncFrequency).map((freq) => (
                      <SelectItem key={freq} value={freq}>
                        {getSyncFrequencyLabel(freq)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
```

- [ ] **Step 2: Remove the standalone "Sync Frequency" Card from the Collapsible**

Find and delete this block inside `<CollapsibleContent>` (around line ~524):

```tsx
          {config.isConfigured && (
            <Card>
              <CardContent className="flex items-center justify-between py-4">
                <div>
                  <div className="font-medium">Sync Frequency</div>
                  <div className="text-sm text-muted-foreground">
                    How often to automatically sync
                  </div>
                </div>
                <Select
                  value={effectiveFrequency}
                  onValueChange={(value) => handleFrequencyChange(value as SyncFrequency)}
                  disabled={isUpdating}
                >
                  <SelectTrigger className="w-[160px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.values(SyncFrequency).map((freq) => (
                      <SelectItem key={freq} value={freq}>
                        {getSyncFrequencyLabel(freq)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </CardContent>
            </Card>
          )}
```

- [ ] **Step 3: Check for unused imports**

After removing the standalone Card, verify `CardContent` is still used elsewhere in the file. If it is not, remove it from the import at the top:

```tsx
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
```

becomes (if `CardContent` is no longer used):

```tsx
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
```

Run the linter to confirm:

```bash
cd ui/frontend && npx eslint --fix src/routes/_authenticated/sync/\$storefront.tsx
```

- [ ] **Step 4: Typecheck**

```bash
cd ui/frontend && npm run check
```

Expected: zero errors.

- [ ] **Step 5: Commit**

```bash
git add ui/frontend/src/routes/_authenticated/sync/\$storefront.tsx
git commit -m "feat: move sync schedule into header card (#671)"
```

---

### Task 3: Visual verification

**Files:**
- No file changes

- [ ] **Step 1: Build the frontend**

```bash
make frontend
```

- [ ] **Step 2: Start the server**

Ensure `DATABASE_URL` and `DB_ENCRYPTION_KEY` are set, then:

```bash
./nexorious serve
```

- [ ] **Step 3: Verify configured storefront**

Open a browser and navigate to a configured storefront detail page (e.g. `/sync/steam`).

Confirm:
- The sync frequency dropdown appears in the top-right of the header card, below the "Connected" badge.
- Changing the dropdown value shows a success toast and the new value persists after page refresh.
- Expanding the connection section (click the badge) no longer reveals a separate "Sync Frequency" card.

- [ ] **Step 4: Verify not-configured storefront**

Navigate to a storefront that is not yet configured (e.g. `/sync/gog` if not set up).

Confirm:
- Only the "Not Configured" badge appears in the top-right — no frequency dropdown.

---

### Task 4: Create PR

**Files:**
- No file changes

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feat/issue-671-sync-schedule-header
```

- [ ] **Step 2: Open a PR**

```bash
gh pr create \
  --title "feat: move sync schedule into header card" \
  --body "$(cat <<'EOF'
Closes #671

Moves the sync frequency dropdown from the collapsible connection section into the Platform Header Card on the storefront detail page. The dropdown now appears stacked below the connection status badge, making it immediately visible without having to expand the connection section.

No logic changes — existing `effectiveFrequency`, `handleFrequencyChange`, and `isUpdating` state are reused as-is.
EOF
)"
```
