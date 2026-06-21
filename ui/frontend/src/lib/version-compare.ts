// Minimal X.Y.Z semver helpers for the "What's new" indicator. The server is
// the source of truth for slicing; this only gates the sidebar dot client-side.
const RELEASE_RE = /^v?(\d+)\.(\d+)\.(\d+)$/;

function parse(v: string): [number, number, number] | null {
  const m = RELEASE_RE.exec(v.trim());
  if (!m) return null;
  return [Number(m[1]), Number(m[2]), Number(m[3])];
}

export function isValidRelease(v: string | undefined | null): boolean {
  return !!v && parse(v) !== null;
}

// isNewer reports whether `current` is a strictly newer release than `other`.
// Returns false if either side is not a valid X.Y.Z release.
export function isNewer(current: string, other: string): boolean {
  const a = parse(current);
  const b = parse(other);
  if (!a || !b) return false;
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] > b[i];
  }
  return false;
}
