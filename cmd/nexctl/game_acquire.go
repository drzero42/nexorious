package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
)

func newGameAcquireCmd() *cobra.Command {
	var platform, storefront, ownership string
	cmd := &cobra.Command{
		Use:   "acquire <ref>",
		Short: "Acquire a wishlisted game: move it into the library with a copy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if platform == "" {
				return fmt.Errorf("--platform is required")
			}
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			c := cliclient.New(p.URL)
			u, err := resolveUserGameRef(cmd, c, p.Key, args[0])
			if err != nil {
				return err
			}
			pl, sf := clientops.SplitPlatform(platform)
			if storefront != "" {
				sf = storefront
			}
			if err := clientops.ValidatePlatform(cmd.Context(), c, p.Key, pl); err != nil {
				return err
			}
			own := ownership
			if own == "" {
				own = "owned"
			}
			if _, err := c.MoveToLibrary(cmd.Context(), p.Key, u.ID, []cliclient.PlatformInput{{Platform: pl, Storefront: sf, OwnershipStatus: own}}); err != nil {
				return fmt.Errorf("acquire failed: %w", err)
			}
			fmt.Fprintf(out, "Moved %q to your library.\n", u.Title())
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&platform, "platform", "", "Platform slug, optionally platform/storefront (required)")
	f.StringVar(&storefront, "storefront", "", "Storefront slug (overrides platform/storefront)")
	f.StringVar(&ownership, "ownership", "owned", "Ownership status")
	return cmd
}
