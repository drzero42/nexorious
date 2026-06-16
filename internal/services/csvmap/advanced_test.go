package csvmap

import "testing"

func TestExtractStatusFlags_PrecedenceAndDefault(t *testing.T) {
	sf := &StatusFlags{
		Rules: []FlagRule{
			{Column: "Dominated", Truthy: []string{"1"}, Status: "dominated"},
			{Column: "Mastered", Truthy: []string{"1"}, Status: "mastered"},
			{Column: "Finished", Truthy: []string{"1"}, Status: "completed"},
			{Column: "Shelved", Truthy: []string{"1"}, Status: "dropped"},
			{Column: "Playing", Truthy: []string{"1"}, Status: "in_progress"},
			{Column: "Played", Truthy: []string{"1"}, Status: "shelved"},
		},
		Default: "not_started",
	}
	header := []string{"Played", "Playing", "Finished", "Mastered", "Dominated", "Shelved"}
	idx := buildIndex(header)
	set := func(cols ...string) []string {
		rec := make([]string, len(header))
		for _, c := range cols {
			rec[idx[normKey(c)]] = "1"
		}
		return rec
	}
	cases := []struct {
		on   []string
		want string
	}{
		{nil, "not_started"},
		{[]string{"Played"}, "shelved"},
		{[]string{"Played", "Playing"}, "in_progress"},
		{[]string{"Shelved"}, "dropped"},
		{[]string{"Finished"}, "completed"},
		{[]string{"Mastered", "Finished"}, "mastered"},
		{[]string{"Dominated", "Mastered"}, "dominated"},
		{[]string{"Shelved", "Playing"}, "dropped"},
		{[]string{"Finished", "Shelved"}, "completed"},
	}
	for i, c := range cases {
		if got := extractStatusFlags(set(c.on...), idx, sf); got != c.want {
			t.Errorf("case %d (%v): got %q, want %q", i, c.on, got, c.want)
		}
	}
}

func TestParseHMM(t *testing.T) {
	cases := []struct {
		in   string
		want *float64
	}{
		{"148:00", ptrF(148)},
		{"10:30", ptrF(10.5)},
		{" 5 : 30 ", ptrF(5.5)},
		{"", nil},
		{"abc", nil},
		{"1:2:3", nil},
		{"0:00", nil},
	}
	for _, c := range cases {
		got := parseHMM(c.in)
		switch {
		case c.want == nil && got != nil:
			t.Errorf("parseHMM(%q) = %v, want nil", c.in, *got)
		case c.want != nil && (got == nil || *got != *c.want):
			t.Errorf("parseHMM(%q) = %v, want %v", c.in, got, *c.want)
		}
	}
}

func ptrF(f float64) *float64 { return &f }
