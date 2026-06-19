package cliclient

import (
	"mime"
	"path/filepath"
)

// filenameFromContentDisposition extracts a safe basename from a
// Content-Disposition header value, or "" when the header is absent,
// unparseable, or carries no filename. The result is always a bare basename
// (directory components are stripped) so a server-supplied name can never be
// used to write outside the intended directory.
func filenameFromContentDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	name := filepath.Base(params["filename"])
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}
