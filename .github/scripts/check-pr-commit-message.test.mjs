import assert from 'node:assert/strict';
import { test } from 'node:test';
import { parser } from '@conventional-commits/parser';
import { releaseRelevant, squashMessage, wrapLikeGitHub } from './check-pr-commit-message.mjs';

test('keeps ≤72-char lines verbatim, re-fills longer ones without indent', () => {
  const short = '  _, gclb1, _ := net.ParseCIDR("35.191.0.0/16")';
  const long =
    '  e.SchemeExtractor = echo.ExtractSchemeFromHeaders(echo.TrustIPRange(gclb1), echo.TrustIPRange(gclb2))';
  assert.equal(
    wrapLikeGitHub(`${short}\n${long}`),
    `${short}\ne.SchemeExtractor =\necho.ExtractSchemeFromHeaders(echo.TrustIPRange(gclb1),\necho.TrustIPRange(gclb2))`,
  );
});

// #1221: the raw body parses, the wrapped squash commit does not.
test('catches a body that only breaks the parser after wrapping', () => {
  const body =
    '```go\n  e.SchemeExtractor = echo.ExtractSchemeFromHeaders(echo.TrustIPRange(gclb1), echo.TrustIPRange(gclb2))\n```';
  assert.doesNotThrow(() => parser(`fix: x (#1)\n\n${body}`));
  assert.throws(() => parser(squashMessage('fix: x', body, '1')));
});

test('a BEGIN_COMMIT_OVERRIDE block replaces the message', () => {
  const body = 'BEGIN_COMMIT_OVERRIDE\nfix(deps): update go non-major\nEND_COMMIT_OVERRIDE\n\nfoo(bar(baz))';
  assert.equal(squashMessage('fix: x', body, '1'), 'fix(deps): update go non-major');
});

test('only release-relevant commits are checked', () => {
  assert.ok(releaseRelevant('fix(deps): update go non-major', ''));
  assert.ok(releaseRelevant('feat: x', ''));
  assert.ok(releaseRelevant('chore!: drop old config', ''));
  assert.ok(releaseRelevant('chore(deps)!: x', ''));
  assert.ok(releaseRelevant('chore: x', 'foo\n\nBREAKING CHANGE: y'));
  assert.ok(releaseRelevant('chore: x', 'Release-As: 1.0.0'));
  assert.ok(!releaseRelevant('chore(deps): update anchore/sbom-action action to v0.24.3', 'foo(bar(baz))'));
  assert.ok(!releaseRelevant('ci: x', 'mentions BREAKING CHANGE: mid-line only'));
});
