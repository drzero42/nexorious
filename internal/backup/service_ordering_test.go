package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestCreateTarGz_ManifestFirst verifies that createTarGz writes manifest.json
// as the first regular-file entry in the archive. This keeps listing cheap:
// readManifestFromArchive stops at the first manifest.json, so placing it first
// means reading it only decompresses a few KB instead of advancing past the
// entire database.sql + cover art (gzip is a non-seekable stream). See #983.
func TestCreateTarGz_ManifestFirst(t *testing.T) {
	baseDir := t.TempDir()
	const id = "nexorious-backup-20260101-120000"
	srcDir := filepath.Join(baseDir, id)

	coverDir := filepath.Join(srcDir, "cover_art")
	if err := os.MkdirAll(coverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// Lexically, cover_art < database.sql < manifest.json, so without an explicit
	// ordering manifest.json would be emitted last by filepath.WalkDir.
	if err := os.WriteFile(filepath.Join(coverDir, "art.jpg"), []byte("fake image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "database.sql"), []byte("-- fake dump"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "manifest.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(baseDir, id+".tar.gz")
	if err := createTarGz(archivePath, baseDir, id); err != nil {
		t.Fatalf("createTarGz: %v", err)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)

	var manifestCount int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if manifestCount == 0 && filepath.Base(hdr.Name) != "manifest.json" {
			t.Fatalf("expected manifest.json to be the first regular-file entry, got %q", hdr.Name)
		}
		if filepath.Base(hdr.Name) == "manifest.json" {
			manifestCount++
		}
	}

	if manifestCount != 1 {
		t.Fatalf("expected manifest.json to appear exactly once, got %d", manifestCount)
	}
}
