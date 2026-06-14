// Validate that the squash commit message release-please will see on merge can
// be parsed by its conventional-commit parser.
//
// Why this exists: release-please 17.x uses the strict @conventional-commits
// parser, which THROWS on some perfectly legal free-form commit bodies — e.g.
// attached, nested call-syntax like `foo(bar(baz))`. When it throws,
// release-please logs "commit could not be parsed", counts the commit as
// non-existent, and SILENTLY drops it from the changelog while its own workflow
// run stays green. PR #985 hit exactly this and never appeared in any release's
// notes. There is no upstream fix (parser is pinned at its latest, 0.4.1; the
// bug is googleapis/release-please#2564, open).
//
// Because the repo sets squash_merge_commit_message=PR_BODY, the message
// release-please parses is "<PR title> (#<number>)\n\n<PR body>". We reconstruct
// that here and run the exact same parser, so an unparseable message fails the
// PR loudly instead of vanishing silently after merge.
import { parser } from '@conventional-commits/parser';

const title = process.env.PR_TITLE ?? '';
const body = process.env.PR_BODY ?? '';
const number = process.env.PR_NUMBER ?? '';

const subject = number ? `${title} (#${number})` : title;
const message = body.trim() ? `${subject}\n\n${body}` : subject;

try {
  parser(message);
  console.log('Squash commit message parses cleanly.');
} catch (e) {
  const first = String(e?.message ?? e).split('\n')[0];
  console.error(
    "::error::release-please cannot parse this PR's squash commit message and " +
      'would silently drop it from the changelog after merge.',
  );
  console.error(`Parser error: ${first}`);
  console.error(
    'Most common cause: attached nested call-syntax in the body, e.g. ' +
      '`foo(bar(baz))`. Reword that line (add a space, rephrase, or split the ' +
      'parentheses) so the parser accepts it, then re-check.',
  );
  process.exit(1);
}
