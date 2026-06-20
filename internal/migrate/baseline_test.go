package migrate

import (
	"testing"

	bunmigrate "github.com/uptrace/bun/migrate"
)

func slice(names ...string) bunmigrate.MigrationSlice {
	ms := make(bunmigrate.MigrationSlice, len(names))
	for i, n := range names {
		ms[i] = bunmigrate.Migration{Name: n}
	}
	return ms
}

func manifestNames() []string {
	out := make([]string, 0, len(v0171Manifest))
	for n := range v0171Manifest {
		out = append(out, n)
	}
	return out
}

func TestClassify(t *testing.T) {
	full := manifestNames()
	cases := []struct {
		name string
		in   bunmigrate.MigrationSlice
		want adoptDecision
	}{
		{"empty is normal/fresh", slice(), decisionNormal},
		{"baseline present is normal", slice(baselineTimestamp), decisionNormal},
		{"baseline plus post is normal", slice(baselineTimestamp, "20260621000001"), decisionNormal},
		{"exact manifest is adopt", slice(full...), decisionAdopt},
		{"manifest minus one is refuse", slice(full[1:]...), decisionRefuse},
		{"manifest plus stranger is refuse", slice(append(append([]string{}, full...), "29990101000001")...), decisionRefuse},
		{"unknown single row is refuse", slice("19990101000001"), decisionRefuse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.in); got != tc.want {
				t.Fatalf("classify(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
