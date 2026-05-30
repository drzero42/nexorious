package psn

import (
	"regexp"
	"strconv"
)

// durationRE matches ISO 8601 durations of the form PTxHxMxS.
// Days and larger units are not produced by the Sony API and are not supported.
var durationRE = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:\d+S)?$`)

// parseDurationFractionalHours parses an ISO 8601 duration string such as
// "PT340H46M13S" and returns hours as H + M/60. Seconds are intentionally
// dropped — the display layer buckets to half-hours, so second-level
// precision is invisible end-to-end. Returns 0 for unrecognised input.
func parseDurationFractionalHours(s string) float64 {
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])    //nolint:errcheck // optional hours group; empty match yields 0
	mins, _ := strconv.Atoi(m[2]) //nolint:errcheck // optional minutes group; empty match yields 0
	return float64(h) + float64(mins)/60.0
}
