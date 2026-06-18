// Package cliauth holds the API-key login bootstrap shared by `nexctl account
// login` and `nexorious setup --login`.
package cliauth

import (
	"fmt"
	"io"
	"os"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
)

// DefaultServerURL is the URL assumed when none is provided.
const DefaultServerURL = "http://localhost:8000"

// LoginAndStoreKey logs in with the given credentials, rotates out any key
// already stored under profileName, mints a fresh CLI key, drops the throwaway
// session, and saves the key to the named profile (marking it current).
func LoginAndStoreKey(out io.Writer, client *cliclient.Client, cfg *clicfg.Config, profileName, url, username, password string) error {
	sessionID, err := client.Login(username, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Rotate: revoke a previously stored key for this profile before minting a new one.
	if existing, _ := cfg.Profile(profileName); existing.KeyID != "" {
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

	cfg.SetProfile(profileName, clicfg.Profile{
		URL:      url,
		Username: username,
		KeyName:  keyName,
		KeyID:    keyID,
		Key:      key,
	})
	if err := clicfg.Save(cfg); err != nil {
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
