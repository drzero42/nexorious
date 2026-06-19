package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/clicfg"
	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print client and server version information and exit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			url, key := profileTarget(cmd)
			return runVersion(cmd.OutOrStdout(), url, key, flagBool(cmd, "json"))
		},
	}
}

// profileTarget resolves the active (or --profile) profile's server URL and API
// key without requiring a login. Either may be empty: an empty URL means no
// profile is configured, so there is no server to query.
func profileTarget(cmd *cobra.Command) (url, key string) {
	cfg, err := clicfg.Load()
	if err != nil {
		return "", ""
	}
	p, ok := cfg.ProfileNamed(profileName(cmd, cfg))
	if !ok {
		return "", ""
	}
	return p.URL, p.Key
}

// versionOutput is the --json shape: the client build, plus the server build (or
// an error explaining why it could not be fetched).
type versionOutput struct {
	Client clientVersion `json:"client"`
	Server any           `json:"server"`
}

type clientVersion struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// runVersion prints the client version and, when a server URL is configured, the
// server version fetched from GET /api/version. A server that cannot be reached
// is reported inline (never fatal): the command always exits 0.
func runVersion(out io.Writer, url, key string, asJSON bool) error {
	client := clientVersion{Version: version, Commit: commit}

	var srv *cliclient.ServerVersionInfo
	var srvErr error
	if url == "" {
		srvErr = fmt.Errorf("no server configured (run `nexctl account login` first)")
	} else {
		srv, srvErr = cliclient.New(url).ServerVersion(key)
	}

	if asJSON {
		o := versionOutput{Client: client}
		if srvErr != nil {
			o.Server = map[string]string{"error": srvErr.Error()}
		} else {
			o.Server = srv
		}
		return cliui.EncodeJSON(out, o)
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "nexctl\t%s (%s)\n", client.Version, client.Commit)
	if srvErr != nil {
		fmt.Fprintf(tw, "server\tunavailable (%s)\n", srvErr)
	} else {
		fmt.Fprintf(tw, "server\t%s (%s)\n", srv.Version, srv.Commit)
	}
	_ = tw.Flush() //nolint:errcheck // best-effort render to an in-memory/stdout writer
	return nil
}
