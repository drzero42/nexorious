package backup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ErrPsqlTooOld is returned when the installed psql does not know the
// \restrict meta-command, so a dump cannot be loaded without letting it run
// backslash commands.
var ErrPsqlTooOld = errors.New("psql is too old to restore safely: it lacks \\restrict (PostgreSQL 18, or 17.6 / 16.10 / 15.14 / 14.19 / 13.22 or later, is required)")

// restoreTimeoutBase is the minimum time a dump load may take; restoreTimeoutPerMiB
// is added for every MiB of dump so large databases get proportionally longer.
const (
	restoreTimeoutBase   = 5 * time.Minute
	restoreTimeoutPerMiB = 2 * time.Second
)

// restoreTimeout scales the psql deadline with the dump size.
func restoreTimeout(dumpBytes int64) time.Duration {
	return restoreTimeoutBase + time.Duration(dumpBytes/(1<<20))*restoreTimeoutPerMiB
}

// RunPsqlRestore loads a plain-format pg_dump file into the database.
//
// The dump comes from a backup archive, whose contents nothing vouches for, so
// psql runs in restricted mode: the input starts with a \restrict line carrying
// a fresh random key that is never sent back with \unrestrict, which makes psql
// refuse every backslash command in the dump (\!, \gexec, \o, ...). COPY data
// blocks still load. pg_dump's own \restrict/\unrestrict pair is removed while
// streaming (see restrictFilter), since a nested \restrict would be refused.
//
// The load runs in a single transaction with ON_ERROR_STOP, so any error —
// including a refused backslash command — aborts it and leaves nothing behind.
var RunPsqlRestore = func(conn DBConnParams, sqlFilePath string) error {
	f, err := os.Open(sqlFilePath) //nolint:gosec // path is database.sql inside our own extraction temp dir
	if err != nil {
		return fmt.Errorf("open dump: %w", err)
	}
	defer func() { _ = f.Close() }()

	var size int64
	if fi, err := f.Stat(); err == nil {
		size = fi.Size()
	}

	ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout(size))
	defer cancel()

	key := rand.Text()
	pr, pw := io.Pipe()
	go func() {
		_, err := io.WriteString(pw, "\\restrict "+key+"\n")
		if err == nil {
			err = restrictFilter(f, pw)
		}
		_ = pw.CloseWithError(err)
	}()

	cmd := exec.CommandContext(ctx, "psql", //nolint:gosec // fixed binary; only validated DB-connection flag values are interpolated, not the command
		"--no-psqlrc", "--quiet",
		"--set=ON_ERROR_STOP=1",
		"--single-transaction",
		"--host="+conn.Host, "--port="+conn.Port,
		"--username="+conn.User, "--dbname="+conn.DBName,
		"--file=-",
	)
	cmd.Env = append(cmd.Environ(), "PGPASSWORD="+conn.Password)
	cmd.Stdin = pr
	output, err := cmd.CombinedOutput()
	// Unblock the feeder goroutine if psql exited before reading all input.
	_ = pr.CloseWithError(io.ErrClosedPipe)
	if err != nil {
		if strings.Contains(string(output), `invalid command \restrict`) {
			return fmt.Errorf("%w\noutput: %s", ErrPsqlTooOld, output)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("psql restore timed out after %s: %w\noutput: %s", restoreTimeout(size), err, output)
		}
		return fmt.Errorf("psql restore failed: %w\noutput: %s", err, output)
	}
	return nil
}

// dumpRestrictLine matches the \restrict line pg_dump writes into the header of
// a plain dump. pg_dump keys are alphanumeric.
var dumpRestrictLine = regexp.MustCompile(`^\\restrict ([A-Za-z0-9]+)$`)

// restrictFilter copies a plain dump from r to w, dropping only the
// \restrict / \unrestrict pair pg_dump (17.6+ / 18) writes itself:
//
//   - the \restrict <key> line, when it appears before the first SQL statement
//     (only blank and "--" comment lines may precede it);
//   - the \unrestrict <same key> line, when it is the last non-blank line.
//
// Everything else passes through byte for byte. In particular there is no
// pattern-based stripping elsewhere: a COPY data line may legitimately start
// with a backslash escape (e.g. \r), and any other backslash command is left
// for restricted psql to refuse. Dumps without the pair (older pg_dump) pass
// through unchanged.
func restrictFilter(r io.Reader, w io.Writer) error {
	br := bufio.NewReaderSize(r, 64*1024)
	bw := bufio.NewWriterSize(w, 64*1024)

	inHeader := true
	var key string
	// held is a candidate \unrestrict line plus any blank lines after it; it is
	// dropped at EOF and written out if anything non-blank follows.
	var held [][]byte

	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			content := bytes.TrimRight(line, "\r\n")
			blank := len(bytes.TrimSpace(content)) == 0

			switch {
			case inHeader:
				if blank || bytes.HasPrefix(content, []byte("--")) {
					if _, err := bw.Write(line); err != nil {
						return err
					}
					break
				}
				inHeader = false
				if m := dumpRestrictLine.FindSubmatch(content); m != nil {
					key = string(m[1])
					break // drop pg_dump's own \restrict
				}
				if _, err := bw.Write(line); err != nil {
					return err
				}
			case held != nil:
				if blank {
					held = append(held, line)
					break
				}
				for _, h := range held {
					if _, err := bw.Write(h); err != nil {
						return err
					}
				}
				held = nil
				if key != "" && string(content) == `\unrestrict `+key {
					held = [][]byte{line}
					break
				}
				if _, err := bw.Write(line); err != nil {
					return err
				}
			case key != "" && string(content) == `\unrestrict `+key:
				held = [][]byte{line}
			default:
				if _, err := bw.Write(line); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			// A held \unrestrict reached EOF: it was the footer line, drop it.
			return bw.Flush()
		}
		if readErr != nil {
			return readErr
		}
	}
}
