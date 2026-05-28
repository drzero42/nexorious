# Enforce gofmt in CI (issue #632)

## Problem

CI's only Go check is `golangci-lint run`. Its active linters do not enforce formatting, so `gofmt` drift goes undetected. ~26 first-party files are currently unformatted on `main`.

## Goal

`golangci-lint run` fails on any future unformatted Go file. Pre-existing drift is cleaned up first.

## Design

### `.golangci.yml` change

Add a `formatters:` block at the top level:

```yaml
formatters:
  enable:
    - gofmt
```

golangci-lint v2 treats this as a first-class check integrated into `golangci-lint run` — no extra CI step required.

### Cleanup commit

Run `gofmt -w` on the 26 currently-drifted files before enabling the formatter, so CI is green from the first push.

### Local dev

No changes needed. `.claude/hooks/post-edit.sh` already runs `gofmt -w` on every edited `.go` file, keeping local dev in sync with CI automatically.

## Out of scope

- gofumpt (stricter superset) — decided against; gofmt matches what the local hook already uses
- Additional linters — separate concern, not part of this change

## Commit plan

1. `chore: reformat Go files with gofmt` — one-time cleanup of the 26 drifted files
2. `chore(ci): enable gofmt formatter in golangci-lint` — add the `formatters:` block to `.golangci.yml`

## Acceptance criteria

- `formatters: enable: [gofmt]` present in `.golangci.yml`
- All 26 pre-existing unformatted files reformatted
- `golangci-lint run` passes on a clean tree
- `golangci-lint run` fails if any `.go` file is unformatted
