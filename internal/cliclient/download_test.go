package cliclient

import "testing"

func TestFilenameFromContentDisposition(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"typical attachment", `attachment; filename="nexorious-backup-20260102-150405.tar.gz"`, "nexorious-backup-20260102-150405.tar.gz"},
		{"unquoted filename", `attachment; filename=export.csv`, "export.csv"},
		{"empty header", "", ""},
		{"no filename param", "attachment", ""},
		{"unparseable", "=not a valid header=", ""},
		{"strips directory traversal", `attachment; filename="../../etc/passwd"`, "passwd"},
		{"strips leading path", `attachment; filename="/abs/path/file.tar.gz"`, "file.tar.gz"},
		{"bare dot is dropped", `attachment; filename="."`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := filenameFromContentDisposition(tc.header); got != tc.want {
				t.Errorf("filenameFromContentDisposition(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}
