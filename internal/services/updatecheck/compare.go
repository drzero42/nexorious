// Package updatecheck checks GitHub for a newer Nexorious release. The
// periodic worker (internal/scheduler) fetches and stores the result; the
// /api/version handler reads it — no network call in the request path.
package updatecheck

import (
	"strings"

	"golang.org/x/mod/semver"
)

// UpdateAvailable reports whether latest is a strictly newer semver than
// running. Returns false when either side is not valid semver (e.g. a "dev"
// build), so non-release builds never claim an update.
func UpdateAvailable(running, latest string) bool {
	r, l := normalize(running), normalize(latest)
	if !semver.IsValid(r) || !semver.IsValid(l) {
		return false
	}
	return semver.Compare(l, r) > 0
}

// normalize adds the leading "v" that x/mod/semver requires.
func normalize(v string) string {
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// IsValidVersion reports whether v is valid semver (with or without the
// leading "v"). Callers use it to skip update checks for non-release builds
// such as "dev".
func IsValidVersion(v string) bool {
	return semver.IsValid(normalize(v))
}

// Compare returns -1, 0, or +1 as a is less than, equal to, or greater than b,
// comparing as semver (with or without a leading "v"). Invalid versions (e.g.
// "dev") sort before valid ones; two invalid versions compare equal. Callers
// slice the changelog with this so non-release builds degrade gracefully.
func Compare(a, b string) int {
	na, nb := normalize(a), normalize(b)
	va, vb := semver.IsValid(na), semver.IsValid(nb)
	switch {
	case va && vb:
		return semver.Compare(na, nb)
	case va:
		return 1
	case vb:
		return -1
	default:
		return 0
	}
}
