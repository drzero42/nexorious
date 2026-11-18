package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// writeFileAtomic streams r into destPath so that destPath only ever holds a
// complete download: the bytes go to a temp file in the same directory (so the
// rename can't cross filesystems), which is synced and renamed over destPath
// only after the whole body arrived. On any failure — a dropped connection, a
// full disk, Ctrl-C cancelling the request — the temp file is removed and an
// existing file at destPath is left untouched. The file is created 0600: an
// export or backup is the user's private data.
func writeFileAtomic(destPath string, r io.Reader) (err error) {
	f, err := os.CreateTemp(filepath.Dir(destPath), "."+filepath.Base(destPath)+".*.part")
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close() // already closed on the rename path; a second Close is harmless
			if rmErr := os.Remove(tmp); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove partial file %s: %w", tmp, rmErr))
			}
		}
	}()

	if _, err = io.Copy(f, r); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return fmt.Errorf("sync output file: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("close output file: %w", err)
	}
	if err = os.Rename(tmp, destPath); err != nil {
		return fmt.Errorf("move output file into place: %w", err)
	}
	return nil
}
