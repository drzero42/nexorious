package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
)

var errNoSubcommand = errors.New("no subcommand provided")

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "nexctl",
		Short: "Nexorious CLI client — manage a remote collection from the terminal",
		Long: "nexctl is a REST client for a Nexorious server. Authenticate with\n" +
			"`nexctl account login`, then manage your collection, pools, tags, sync,\n" +
			"and more. Use --profile to target one of several configured servers.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Help(); err != nil {
				return err
			}
			return errNoSubcommand
		},
	}

	pf := root.PersistentFlags()
	pf.String("profile", "", "Config profile to use (default: the current profile)")
	pf.Bool("json", false, "Emit machine-readable JSON")
	pf.BoolP("quiet", "q", false, "Emit only bare ids/values for piping")
	pf.BoolP("yes", "y", false, "Skip confirmation prompts")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newAccountCmd())
	root.AddCommand(newProfileCmd())
	root.AddCommand(newGameCmd())
	root.AddCommand(newTagCmd())
	root.AddCommand(newPoolCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newJobCmd())
	root.AddCommand(newImportCmd())
	root.AddCommand(newLoginCmd(), newLogoutCmd()) // top-level aliases for `account login`/`logout`

	return root
}

// profileName returns the --profile value, defaulting to the current profile.
func profileName(cmd *cobra.Command, cfg *clicfg.Config) string {
	name, _ := cmd.Flags().GetString("profile") //nolint:errcheck // absent flag yields ""
	if name == "" {
		name = cfg.CurrentName()
	}
	return name
}

// resolveProfile loads config and returns the active profile, erroring with a
// login hint when no API key is stored for it.
func resolveProfile(cmd *cobra.Command) (clicfg.Profile, *clicfg.Config, error) {
	cfg, err := clicfg.Load()
	if err != nil {
		return clicfg.Profile{}, nil, err
	}
	name := profileName(cmd, cfg)
	p, ok := cfg.Profile(name)
	if !ok || p.Key == "" {
		return clicfg.Profile{}, nil, fmt.Errorf("not logged in to profile %q (run `nexctl account login` first)", name)
	}
	return p, cfg, nil
}

// flagBool reads an inherited persistent bool flag.
func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name) //nolint:errcheck // absent flag yields false
	return v
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		if !errors.Is(err, errNoSubcommand) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}
