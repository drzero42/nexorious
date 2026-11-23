package backup

import (
	"bytes"
	"strings"
	"testing"
)

func TestRestrictFilter(t *testing.T) {
	const key = "AbC123xyz"
	header := "--\n-- PostgreSQL database dump\n--\n\n"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "pg_dump pair is removed",
			in:   header + `\restrict ` + key + "\n\n-- Dumped from 18.6\nSET x = 1;\nCREATE TABLE t (a int);\n\n--\n-- PostgreSQL database dump complete\n--\n\n" + `\unrestrict ` + key + "\n\n",
			want: header + "\n-- Dumped from 18.6\nSET x = 1;\nCREATE TABLE t (a int);\n\n--\n-- PostgreSQL database dump complete\n--\n\n",
		},
		{
			name: "dump without the pair passes through unchanged",
			in:   header + "SET x = 1;\nCREATE TABLE t (a int);\n",
			want: header + "SET x = 1;\nCREATE TABLE t (a int);\n",
		},
		{
			name: "restrict after the first statement is kept",
			in:   header + "SET x = 1;\n" + `\restrict ` + key + "\n",
			want: header + "SET x = 1;\n" + `\restrict ` + key + "\n",
		},
		{
			name: "restrict with a non-alphanumeric key is kept",
			in:   header + `\restrict a;b` + "\nSET x = 1;\n",
			want: header + `\restrict a;b` + "\nSET x = 1;\n",
		},
		{
			name: "only the first header restrict is removed",
			in:   header + `\restrict ` + key + "\n" + `\restrict ` + key + "\nSET x = 1;\n",
			want: header + `\restrict ` + key + "\nSET x = 1;\n",
		},
		{
			name: "unrestrict without a header restrict is kept",
			in:   header + "SET x = 1;\n" + `\unrestrict ` + key + "\n",
			want: header + "SET x = 1;\n" + `\unrestrict ` + key + "\n",
		},
		{
			name: "unrestrict with the wrong key is kept",
			in:   header + `\restrict ` + key + "\nSET x = 1;\n" + `\unrestrict other` + "\n",
			want: header + "SET x = 1;\n" + `\unrestrict other` + "\n",
		},
		{
			name: "unrestrict followed by more input is kept",
			in:   header + `\restrict ` + key + "\nSET x = 1;\n" + `\unrestrict ` + key + "\n\n\\! id\n",
			want: header + "SET x = 1;\n" + `\unrestrict ` + key + "\n\n\\! id\n",
		},
		{
			name: "earlier unrestrict kept, final one removed",
			in:   header + `\restrict ` + key + "\n" + `\unrestrict ` + key + "\nSET x = 1;\n" + `\unrestrict ` + key + "\n",
			want: header + `\unrestrict ` + key + "\nSET x = 1;\n",
		},
		{
			name: "final unrestrict without trailing newline is removed",
			in:   header + `\restrict ` + key + "\nSET x = 1;\n" + `\unrestrict ` + key,
			want: header + "SET x = 1;\n",
		},
		{
			name: "COPY data starting with a backslash escape is untouched",
			in:   header + `\restrict ` + key + "\nCOPY t (s) FROM stdin;\n\\rstart\n\\N\n\\restrict-looking\n\\.\n" + `\unrestrict ` + key + "\n",
			want: header + "COPY t (s) FROM stdin;\n\\rstart\n\\N\n\\restrict-looking\n\\.\n",
		},
		{
			name: "other backslash commands are left for psql to refuse",
			in:   header + `\restrict ` + key + "\n\\! touch /tmp/x\n" + `\unrestrict ` + key + "\n",
			want: header + "\\! touch /tmp/x\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := restrictFilter(strings.NewReader(tc.in), &out); err != nil {
				t.Fatalf("restrictFilter: %v", err)
			}
			if got := out.String(); got != tc.want {
				t.Errorf("output mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestRestrictFilter_LongLine(t *testing.T) {
	// COPY rows can far exceed bufio.Scanner's default token limit.
	long := strings.Repeat("x", 1<<20)
	in := "COPY t (s) FROM stdin;\n" + long + "\n\\.\n"
	var out bytes.Buffer
	if err := restrictFilter(strings.NewReader(in), &out); err != nil {
		t.Fatalf("restrictFilter: %v", err)
	}
	if out.String() != in {
		t.Error("long line was altered")
	}
}
