# React.ElementRef → React.ComponentRef Migration

**Issue:** [#538](https://github.com/drzero42/nexorious/issues/538)
**Date:** 2026-05-19

## Background

React 19 deprecated `React.ElementRef`. The replacement is `React.ComponentRef` — same generic signature, same semantics, new name. TypeScript surfaces this as deprecation hint `[6385]` in the IDE. `tsc --noEmit` (i.e. `npm run check`) still passes today, so there is no CI break, only IDE noise.

The deprecation surfaced during the knip integration work (#516).

## Goal

Remove every `React.ElementRef` reference from the frontend so the deprecation hints disappear, with zero behavior change.

## Scope

A single mechanical rename across all 14 files under `ui/frontend/src/components/ui/` that currently reference `React.ElementRef`. Total occurrences: **38**.

| File                  | Occurrences |
|-----------------------|-------------|
| alert-dialog.tsx      | 6           |
| avatar.tsx            | 3           |
| checkbox.tsx          | 1           |
| command.tsx           | 7           |
| dialog.tsx            | 4           |
| dropdown-menu.tsx     | 2           |
| label.tsx             | 1           |
| popover.tsx           | 1           |
| progress.tsx          | 1           |
| scroll-area.tsx       | 2           |
| select.tsx            | 5           |
| sheet.tsx             | 3           |
| switch.tsx            | 1           |
| tooltip.tsx           | 1           |

The issue body listed 9 files / 32 occurrences but explicitly asked the implementer to verify with `grep -rn "React.ElementRef" ui/frontend/src` first. The verification surfaced 6 additional files (checkbox, label, popover, progress, switch, tooltip); all are included.

## Non-Goals

- No other deprecated React APIs are touched in this change.
- No styling, restructuring, or refactoring of the shadcn components.
- No upstream shadcn upgrade — the components are vendored "include and prune" and stay that way.

## Implementation

Mechanical find-and-replace: `React.ElementRef` → `React.ComponentRef`. The two types share the same generic parameter (`<typeof Primitive.X>` patterns continue to work unchanged), so no surrounding edits are required.

## Verification

All four must pass (run from `ui/frontend/`):

1. `grep -rn "React.ElementRef" ui/frontend/src` returns no matches.
2. `npm run check` — TypeScript compiles cleanly; confirms `ComponentRef` accepts the same generic argument shapes.
3. `npm run knip` — no new dead-code findings introduced.
4. `npm run test` — full Vitest suite passes; confirms no runtime regression for components whose ref types were renamed.

## Risks

Effectively none. `React.ComponentRef` is an exact rename of `React.ElementRef` in React 19's type definitions, and all usages here pass a `typeof PrimitiveComponent` generic argument that both aliases accept identically. The `npm run check` and `npm run test` gates catch any unexpected mismatch.
