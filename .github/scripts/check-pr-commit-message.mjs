// Validate the commit message release-please will see for this PR on merge.
//
// The repo squash-merges with squash_merge_commit_title=PR_TITLE and
// squash_merge_commit_message=BLANK, so the squash commit is just
// "<PR title> (#<number>)" — the PR description never reaches the commit
// (#1208: Renovate's release-notes bodies broke release-please's parser).
// release-please parses that commit, unless the merged PR's description has a
// non-empty BEGIN_COMMIT_OVERRIDE … END_COMMIT_OVERRIDE block, which it reads
// from the PR (not the commit) and uses instead.
//
// Two ways this can go wrong silently, both caught here:
//
// 1. The message doesn't parse. release-please 17.x uses the strict
//    @conventional-commits parser; when it throws, release-please logs "commit
//    could not be parsed" and drops the commit from the changelog while its own
//    run stays green (#985, upstream googleapis/release-please#2564, open).
// 2. A Release-As: or BREAKING CHANGE: line sits in the PR description outside
//    an override block. With a BLANK squash body it never reaches the commit,
//    so it has no effect.
import { parser } from '@conventional-commits/parser';

// The override text release-please would use, mirroring its
// preprocessCommitMessage (src/commit.ts, v17.11.2): text between the first
// BEGIN_COMMIT_OVERRIDE and the following END_COMMIT_OVERRIDE, trimmed. An
// empty override is ignored and the commit message is used.
export function commitOverride(body) {
  return (body.split('BEGIN_COMMIT_OVERRIDE')[1] || '').split('END_COMMIT_OVERRIDE')[0].trim();
}

// The message release-please will parse for this PR.
export function releaseMessage(title, body, number) {
  return commitOverride(body) || (number ? `${title} (#${number})` : title);
}

// Release-As / BREAKING CHANGE lines in the description that are outside an
// override block, and so would be ignored.
export function strandedTrailers(body) {
  const outside = commitOverride(body)
    ? body.replace(/BEGIN_COMMIT_OVERRIDE[\s\S]*?END_COMMIT_OVERRIDE/, '')
    : body;
  return outside.match(/^(BREAKING[ -]CHANGE|Release-As):.*$/gim) ?? [];
}

// Whether release-please dropping this commit would change a release. With the
// default changelog sections only feat/fix/perf/revert are user-facing; any
// other type is hidden, so losing it is harmless — unless it is a breaking
// change, or an override block (which can carry Release-As or a breaking-change
// footer) is present.
export function releaseRelevant(title, body) {
  return (
    /^(feat|fix|perf|revert)(\(.*?\))?!?:/.test(title) ||
    /^\w+(\(.*?\))?!:/.test(title) ||
    commitOverride(body) !== ''
  );
}

if (import.meta.main) {
  const title = process.env.PR_TITLE ?? '';
  const body = process.env.PR_BODY ?? '';

  // Bot PRs (Renovate) embed upstream release notes, which may legitimately
  // contain such lines; they are inert there, not a mistake to flag.
  const stranded = process.env.PR_AUTHOR_TYPE === 'Bot' ? [] : strandedTrailers(body);
  if (stranded.length) {
    console.error(
      '::error::The PR description has a Release-As: / BREAKING CHANGE: line outside a ' +
        'BEGIN_COMMIT_OVERRIDE block. Squash commits carry only the PR title, so it would be ' +
        'ignored. Put the full commit message (title line, blank line, trailer) in ' +
        '"BEGIN_COMMIT_OVERRIDE\\n…\\nEND_COMMIT_OVERRIDE" in the description, or use `!` in the ' +
        'title for a breaking change.',
    );
    for (const line of stranded) console.error(`  ${line}`);
    process.exit(1);
  }

  if (!releaseRelevant(title, body)) {
    console.log('Not a release-relevant commit (no feat/fix/perf/revert, breaking change or override); skipping.');
    process.exit(0);
  }
  const message = releaseMessage(title, body, process.env.PR_NUMBER ?? '');
  try {
    parser(message);
    console.log('Release commit message parses cleanly.');
  } catch (e) {
    const first = String(e?.message ?? e).split('\n')[0];
    console.error(
      '::error::release-please cannot parse the commit message for this PR and would ' +
        'silently drop it from the changelog after merge.',
    );
    console.error(`Parser error: ${first}`);
    console.error(
      commitOverride(body)
        ? 'Parsed the BEGIN_COMMIT_OVERRIDE text in the description. release-please treats ANY ' +
            'occurrence of that word as the start of an override, even in prose or backticks — ' +
            'rephrase the description if you only meant to mention it.'
        : 'Fix the PR title, then re-check.',
    );
    process.exit(1);
  }
}
