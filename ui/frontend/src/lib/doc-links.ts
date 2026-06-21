import { GITHUB_REPO_URL } from './repo';

// Base GitHub location for the repo's docs/ directory. Relative links inside a
// guide are resolved against this so that sibling *.md files become in-app
// /help routes and anything outside docs/ (e.g. ../DEV.md) becomes a GitHub
// source link. See issue #887.
const GITHUB_DOCS_BASE = `${GITHUB_REPO_URL}/blob/main/docs/`;

export type ResolvedDocHref =
  | { type: 'internal'; slug: string; hash: string | undefined }
  | { type: 'anchor'; value: string }
  | { type: 'external'; value: string };

/**
 * Classify a Markdown link target found inside the doc identified by
 * `currentSlug`:
 *  - same-page `#anchor` -> in-page scroll
 *  - absolute URL (scheme:) -> external (new tab)
 *  - relative link resolving to `docs/<slug>.md` -> internal `/help/<slug>`
 *  - any other relative link (e.g. `../DEV.md`) -> external GitHub source URL
 */
export function resolveDocHref(href: string, currentSlug: string): ResolvedDocHref {
  if (!href) return { type: 'external', value: '' };
  if (href.startsWith('#')) return { type: 'anchor', value: href };
  // Any explicit scheme (http:, https:, mailto:, etc.) is external as-is.
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) return { type: 'external', value: href };

  const base = `${GITHUB_DOCS_BASE}${currentSlug}.md`;
  let url: URL;
  try {
    url = new URL(href, base);
  } catch {
    return { type: 'external', value: href };
  }
  const m = url.pathname.match(/\/blob\/main\/docs\/([a-z0-9-]+)\.md$/);
  if (m) {
    return { type: 'internal', slug: m[1], hash: url.hash || undefined };
  }
  return { type: 'external', value: url.href };
}
