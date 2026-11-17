import assert from 'node:assert/strict';
import { test } from 'node:test';
import { commitOverride, releaseMessage, releaseRelevant, strandedTrailers } from './check-pr-commit-message.mjs';

test('without an override, the message is the title plus PR number, body ignored', () => {
  assert.equal(releaseMessage('fix(deps): update go non-major', 'foo(bar(baz))\n\nlots of notes', '1227'), 'fix(deps): update go non-major (#1227)');
});

test('a BEGIN_COMMIT_OVERRIDE block replaces the message', () => {
  const body = 'BEGIN_COMMIT_OVERRIDE\nfix: x\n\nRelease-As: 1.2.0\nEND_COMMIT_OVERRIDE\n\nfoo(bar(baz))';
  assert.equal(releaseMessage('fix: x', body, '1'), 'fix: x\n\nRelease-As: 1.2.0');
});

// #1208 follow-up: Renovate rendered `{{{prTitle}}}` empty. release-please
// ignores an empty override and falls back to the commit, so must we.
test('an empty override block is ignored', () => {
  const body = 'BEGIN_COMMIT_OVERRIDE\n\nEND_COMMIT_OVERRIDE';
  assert.equal(commitOverride(body), '');
  assert.equal(releaseMessage('fix(deps): update go non-major', body, '1227'), 'fix(deps): update go non-major (#1227)');
  assert.ok(!releaseRelevant('chore(deps): x', body));
});

test('trailers outside an override block are reported as stranded', () => {
  assert.deepEqual(strandedTrailers('Some text\n\nRelease-As: 2.0.0'), ['Release-As: 2.0.0']);
  assert.deepEqual(strandedTrailers('BREAKING CHANGE: drops X'), ['BREAKING CHANGE: drops X']);
  assert.deepEqual(strandedTrailers('mentions BREAKING CHANGE: mid-line only'), []);
  assert.deepEqual(strandedTrailers('BEGIN_COMMIT_OVERRIDE\nfeat: x\n\nRelease-As: 2.0.0\nEND_COMMIT_OVERRIDE'), []);
  assert.deepEqual(
    strandedTrailers('BEGIN_COMMIT_OVERRIDE\nfeat: x\nEND_COMMIT_OVERRIDE\n\nRelease-As: 2.0.0'),
    ['Release-As: 2.0.0'],
  );
});

test('only release-relevant commits are parsed', () => {
  assert.ok(releaseRelevant('fix(deps): update go non-major', ''));
  assert.ok(releaseRelevant('feat: x', ''));
  assert.ok(releaseRelevant('chore!: drop old config', ''));
  assert.ok(releaseRelevant('chore(deps)!: x', ''));
  assert.ok(releaseRelevant('chore: x', 'BEGIN_COMMIT_OVERRIDE\nchore: x\n\nRelease-As: 1.0.0\nEND_COMMIT_OVERRIDE'));
  assert.ok(!releaseRelevant('chore(deps): update anchore/sbom-action action to v0.24.3', 'foo(bar(baz))'));
  assert.ok(!releaseRelevant('ci: x', 'Release-As: 1.0.0'));
});
