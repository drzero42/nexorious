package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
)

func newGameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "game",
		Short: "Manage your game collection",
	}
	cmd.AddCommand(newGameListCmd())
	cmd.AddCommand(newGameShowCmd())
	cmd.AddCommand(newGameAddCmd())
	cmd.AddCommand(newGameAcquireCmd())
	cmd.AddCommand(newGameRmCmd())
	cmd.AddCommand(newGameEditCmd())
	cmd.AddCommand(newGameStatsCmd())
	cmd.AddCommand(newGameFiltersCmd())
	return cmd
}

// resolveUserGameRef is the CLI wrapper: one hit is used; many prompt a picker
// (interactive) or error with candidate ids (off-TTY).
func resolveUserGameRef(cmd *cobra.Command, c *cliclient.Client, key, ref string) (*cliclient.UserGame, error) {
	games, err := clientops.FindUserGamesByRef(cmd.Context(), c, key, ref)
	if err != nil {
		return nil, err
	}
	switch len(games) {
	case 0:
		return nil, fmt.Errorf("no game matching %q in your library", ref)
	case 1:
		return &games[0], nil
	}
	if interactive(cmd) {
		return pickUserGame(cmd, games)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d games; re-run with an id:", ref, len(games))
	for i := range games {
		fmt.Fprintf(&b, "\n  %s  %s", games[i].ID, games[i].Title())
	}
	return nil, fmt.Errorf("%s", b.String())
}

// interactive reports whether rich prompts are allowed: a TTY and none of
// --json/--quiet/--yes set.
func interactive(cmd *cobra.Command) bool {
	if flagBool(cmd, "json") || flagBool(cmd, "quiet") || flagBool(cmd, "yes") {
		return false
	}
	// Probe the command's own input (honors cmd.SetIn in tests); a non-*os.File
	// reader (e.g. a piped bytes.Reader) is never a TTY.
	if f, ok := cmd.InOrStdin().(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}

func pickUserGame(cmd *cobra.Command, games []cliclient.UserGame) (*cliclient.UserGame, error) {
	out := cmd.OutOrStdout()
	for i, u := range games {
		fmt.Fprintf(out, "%2d) %s  [%s]\n", i+1, u.Title(), statusOf(&games[i]))
	}
	fmt.Fprint(out, "Select a game [1]: ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n') //nolint:errcheck // empty/EOF -> default selection
	choice := strings.TrimSpace(line)
	if choice == "" {
		return &games[0], nil
	}
	n, err := strconv.Atoi(choice)
	if err != nil || n < 1 || n > len(games) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return &games[n-1], nil
}

func statusOf(u *cliclient.UserGame) string {
	if u.IsWishlisted {
		return "wishlist"
	}
	if u.PlayStatus == nil || *u.PlayStatus == "" {
		return "-"
	}
	return *u.PlayStatus
}

func ratingOf(u *cliclient.UserGame) string {
	if u.PersonalRating == nil {
		return "-"
	}
	return strconv.Itoa(*u.PersonalRating)
}

func platformsOf(u *cliclient.UserGame) string {
	if len(u.Platforms) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(u.Platforms))
	for _, p := range u.Platforms {
		if p.Platform != nil {
			parts = append(parts, *p.Platform)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func tagsOf(u *cliclient.UserGame) string {
	if len(u.Tags) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(u.Tags))
	for _, t := range u.Tags {
		parts = append(parts, t.Name)
	}
	return strings.Join(parts, ",")
}
