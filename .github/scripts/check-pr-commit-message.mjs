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
// release-please parses is "<PR title> (#<number>)\n\n<PR body>" — but GitHub
// hard-wraps the body at 72 columns when it builds the squash commit, so we
// must parse the *wrapped* text. #1221 passed this check on its raw body, then
// wrapping moved `echo.Foo(echo.Bar(x), …)` from an indented line to column 0,
// where the parser throws, and the fix was dropped from v0.97.6's changelog.
// We reconstruct that message and run the exact same parser, so an unparseable
// message fails the PR loudly instead of vanishing silently after merge.
import { parser } from '@conventional-commits/parser';

// GitHub's squash-body wrap (verified line-for-line against #1221's squash
// commit): lines ≤72 chars (code points) are kept verbatim; longer lines are
// greedily re-filled at 72 on whitespace, losing their indentation. A single
// word longer than 72 is never split.
const WRAP = 72;
const len = (s) => [...s].length;

export function wrapLikeGitHub(text) {
  return text
    .replace(/\r\n/g, '\n')
    .split('\n')
    .flatMap((line) => {
      if (len(line) <= WRAP) return [line];
      const out = [];
      let cur = '';
      for (const word of line.trim().split(/\s+/)) {
        if (!cur) cur = word;
        else if (len(cur) + 1 + len(word) <= WRAP) cur += ` ${word}`;
        else {
          out.push(cur);
          cur = word;
        }
      }
      if (cur) out.push(cur);
      return out;
    })
    .join('\n');
}

// The message release-please will parse for this PR. A BEGIN_COMMIT_OVERRIDE
// block in the PR body replaces the commit message entirely (release-please
// reads merged PR bodies), so honour it — it's also the escape hatch for
// generated bodies (e.g. Renovate release notes) that can't easily be reworded.
export function squashMessage(title, body, number) {
  const override = body.match(/BEGIN_COMMIT_OVERRIDE\r?\n([\s\S]*?)\r?\nEND_COMMIT_OVERRIDE/);
  if (override) return override[1].trim();
  const subject = number ? `${title} (#${number})` : title;
  return body.trim() ? `${subject}\n\n${wrapLikeGitHub(body)}` : subject;
}

// Whether release-please dropping this commit would change a release. With the
// default changelog sections only feat/fix/perf/revert are user-facing; any
// other type is hidden, so losing it is harmless — unless the commit carries a
// breaking-change marker or a Release-As trailer, which only take effect if the
// message parses.
export function releaseRelevant(title, body) {
  return (
    /^(feat|fix|perf|revert)(\(.*?\))?!?:/.test(title) ||
    /^\w+(\(.*?\))?!:/.test(title) ||
    /^(BREAKING[ -]CHANGE|Release-As):/im.test(body)
  );
}

if (import.meta.main) {
  const title = process.env.PR_TITLE ?? '';
  const body = process.env.PR_BODY ?? '';
  if (!releaseRelevant(title, body)) {
    console.log('Not a release-relevant commit (no feat/fix/perf/revert, breaking change or Release-As); skipping.');
    process.exit(0);
  }
  const message = squashMessage(title, body, process.env.PR_NUMBER ?? '');
  try {
    parser(message);
    console.log('Squash commit message parses cleanly.');
  } catch (e) {
    const first = String(e?.message ?? e).split('\n')[0];
    console.error(
      "::error::release-please cannot parse this PR's squash commit message and " +
        'would silently drop it from the changelog after merge.',
    );
    console.error(`Parser error (line numbers are after GitHub's 72-column wrap): ${first}`);
    console.error(
      'Most common cause: attached nested call-syntax in the body, e.g. ' +
        '`foo(bar(baz))`, landing at the start of a wrapped line. Reword that ' +
        'line (add a space, rephrase, or split the parentheses), or put ' +
        '"BEGIN_COMMIT_OVERRIDE\\n<conventional commit message>\\nEND_COMMIT_OVERRIDE" ' +
        'at the top of the PR body, then re-check.',
    );
    process.exit(1);
  }
}
