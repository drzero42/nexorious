// Package cliui holds front-end-agnostic terminal helpers shared by the
// nexorious and nexctl binaries: TTY detection, prompts, confirmation, and
// machine-readable output.
package cliui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// FirstNonEmpty returns the first non-empty string, or "".
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Prompt writes label to out and reads one trimmed line from in.
func Prompt(in *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// ReadPassword resolves a password from the NEXORIOUS_PASSWORD env var, else a
// no-echo TTY prompt, else a plain line from in (piped input). The non-TTY path
// reuses the caller's reader so it does not race with other prompts on stdin.
func ReadPassword(in *bufio.Reader, out io.Writer) (string, error) {
	if env := os.Getenv("NEXORIOUS_PASSWORD"); env != "" {
		return env, nil
	}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(out, "Password: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// Confirm asks a yes/no question. When assumeYes is true it returns true without
// prompting. Anything other than y/yes (case-insensitive) is false.
func Confirm(in *bufio.Reader, out io.Writer, question string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	fmt.Fprintf(out, "%s [y/N] ", question)
	line, _ := in.ReadString('\n') //nolint:errcheck // EOF/partial line still yields the typed answer
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// EncodeJSON writes v as indented JSON followed by a newline.
func EncodeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
