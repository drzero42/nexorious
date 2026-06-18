package db

import (
	"errors"
	"testing"
)

func TestIsUniqueViolation(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"sqlstate 23505", errors.New(`ERROR: duplicate key value violates unique constraint "x" (SQLSTATE 23505)`), true},
		{"unique constraint text", errors.New("unique constraint failed"), true},
		{"unique_violation text", errors.New("pq: unique_violation"), true},
		{"unrelated", errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUniqueViolation(tc.err); got != tc.want {
				t.Fatalf("IsUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
