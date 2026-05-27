package psn

import (
	"regexp"
	"strconv"
)

// durationRE matches ISO 8601 durations of the form PTxHxMxS.
// Days and larger units are not produced by the Sony API and are not supported.
var durationRE = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:\d+S)?$`)

// parseDurationHours extracts the hours component from an ISO 8601 duration string
// such as "PT340H46M13S". Truncates; does not round. Returns 0 for unrecognised input.
func parseDurationHours(s string) int {
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1]) //nolint:errcheck // optional hours group; empty match yields 0, which is correct
	return h
}
