package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

const defaultServerURL = "http://localhost:8000"

// newLoginCmd returns the `login` subcommand.
func newLoginCmd() *cobra.Command {
	var urlFlag, usernameFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate to a Nexorious server and store an API key",
		Long: "Exchange a username and password for an API key and store it in the\n" +
			"local CLI config file. Subsequent commands use the stored key. The\n" +
			"password is read from the NEXORIOUS_PASSWORD environment variable when\n" +
			"set, otherwise prompted for interactively.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, urlFlag, usernameFlag)
		},
	}
	cmd.Flags().StringVar(&urlFlag, "url", "", "Server URL (prompted if omitted)")
	cmd.Flags().StringVar(&usernameFlag, "username", "", "Username (prompted if omitted)")
	return cmd
}

func runLogin(cmd *cobra.Command, urlFlag, usernameFlag string) error {
	cfg, err := clicfg.Load()
	if err != nil {
		return err
	}
	existing, _ := cfg.CurrentProfile()

	in := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	url := firstNonEmpty(urlFlag, existing.URL)
	if url == "" {
		url, err = prompt(in, out, fmt.Sprintf("Server URL [%s]: ", defaultServerURL))
		if err != nil {
			return err
		}
		url = firstNonEmpty(url, defaultServerURL)
	}

	username := firstNonEmpty(usernameFlag, existing.Username)
	if username == "" {
		username, err = prompt(in, out, "Username: ")
		if err != nil {
			return err
		}
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}

	password, err := readPassword(in, out)
	if err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	client := cliclient.New(url)

	sessionID, err := client.Login(username, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Rotate: revoke the previously stored key (if any) before minting a new one.
	if existing.KeyID != "" {
		if err := client.RevokeAPIKeyWithCookie(sessionID, existing.KeyID); err != nil {
			fmt.Fprintf(out, "warning: could not revoke previous key %s: %v\n", existing.KeyID, err)
		}
	}

	keyName := "cli@" + hostname()
	key, keyID, err := client.CreateAPIKey(sessionID, keyName)
	if err != nil {
		return fmt.Errorf("create API key failed: %w", err)
	}

	// Drop the throwaway session; failure here is non-fatal.
	if err := client.Logout(sessionID); err != nil {
		fmt.Fprintf(out, "warning: could not close bootstrap session: %v\n", err)
	}

	cfg.SetProfile(cfg.CurrentName(), clicfg.Profile{
		URL:      url,
		Username: username,
		KeyName:  keyName,
		KeyID:    keyID,
		Key:      key,
	})
	if err := clicfg.Save(cfg); err != nil {
		// The key was already minted server-side. Surface its id so the user can
		// revoke the now-orphaned key (it is not recorded locally) from the web UI.
		return fmt.Errorf("API key %q (id %s) was created but saving config failed; "+
			"revoke it from the web UI to avoid an orphaned key: %w", keyName, keyID, err)
	}

	fmt.Fprintf(out, "Logged in to %s as %s.\nStored API key %q (%s)", url, username, keyName, maskKey(key))
	if path, err := clicfg.Path(); err == nil {
		fmt.Fprintf(out, " in %s", path)
	}
	fmt.Fprintln(out, ".")
	return nil
}

// readPassword reads the password from NEXORIOUS_PASSWORD, or prompts without
// echo on a TTY, or reads a plain line from the provided reader when not a TTY
// (e.g. piped input). The non-TTY path reuses the caller's reader so it does not
// race with the URL/username prompts over os.Stdin.
func readPassword(in *bufio.Reader, out io.Writer) (string, error) {
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

func prompt(in *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown-host"
	}
	return h
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "…" + key[len(key)-4:]
}
