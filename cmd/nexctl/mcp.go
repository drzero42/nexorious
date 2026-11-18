package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/cliui"
)

// mcpMaxMessage caps one newline-delimited JSON-RPC message read from stdin.
const mcpMaxMessage = 16 << 20

// jsonrpcServerError is the JSON-RPC "server error" code the pump uses for
// failures below the protocol (HTTP status, unreachable server).
const jsonrpcServerError = -32000

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Connect MCP clients to the server's MCP endpoint"}
	cmd.AddCommand(newMCPConfigCmd(), newMCPServeCmd())
	return cmd
}

func newMCPConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the agent-config stanzas (stdio and HTTP) for the active profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, _, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			args := []string{"mcp", "serve"}
			if name, _ := cmd.Flags().GetString("profile"); name != "" { //nolint:errcheck // absent flag yields ""
				args = append(args, "--profile", name)
			}
			stanza := map[string]any{
				"mcpServers": map[string]any{
					"nexorious": map[string]any{"command": "nexctl", "args": args},
					"nexorious-http": map[string]any{
						"type":    "http",
						"url":     strings.TrimRight(p.URL, "/") + "/api/mcp",
						"headers": map[string]string{"Authorization": "Bearer " + p.Key},
					},
				},
			}
			fmt.Fprintln(cmd.ErrOrStderr(), "warning: the nexorious-http stanza contains your API key; keep it secret")
			return cliui.EncodeJSON(cmd.OutOrStdout(), stanza)
		},
	}
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Bridge a stdio MCP client to the server's /api/mcp endpoint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, cfg, err := resolveProfile(cmd)
			if err != nil {
				return err
			}
			pump := mcpPump{
				client:  cliclient.New(p.URL),
				key:     p.Key,
				profile: profileName(cmd, cfg),
				out:     cmd.OutOrStdout(),
				errOut:  cmd.ErrOrStderr(),
			}
			return pump.run(cmd.Context(), cmd.InOrStdin())
		},
	}
}

// mcpPump relays newline-delimited JSON-RPC messages from stdin to POST
// /api/mcp and writes each response back as one line. It never parses the
// protocol beyond peeking at a message's id (to answer HTTP-level failures),
// so a server protocol upgrade never needs a nexctl upgrade.
type mcpPump struct {
	client  *cliclient.Client
	key     string
	profile string
	out     io.Writer
	errOut  io.Writer
}

func (p mcpPump) run(ctx context.Context, in io.Reader) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), mcpMaxMessage)
	for sc.Scan() {
		msg := bytes.TrimSpace(sc.Bytes())
		if len(msg) == 0 {
			continue
		}
		if err := p.relay(ctx, msg); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	return nil
}

// relay forwards one message and writes its reply. Only a stdout write
// failure is returned; HTTP-level failures become JSON-RPC error replies.
func (p mcpPump) relay(ctx context.Context, msg []byte) error {
	resp, err := p.client.PostMCP(ctx, p.key, msg)
	if err != nil {
		id, needsReply := messageID(msg)
		text := p.failureMessage(err)
		if !needsReply {
			fmt.Fprintf(p.errOut, "nexctl mcp: dropped notification: %s\n", text)
			return nil
		}
		return p.writeError(id, text)
	}
	if len(bytes.TrimSpace(resp.Body)) == 0 {
		return nil // 202: reply to a notification
	}
	if strings.HasPrefix(resp.ContentType, "text/event-stream") {
		for _, data := range sseData(resp.Body) {
			if err := p.writeLine(data); err != nil {
				return err
			}
		}
		return nil
	}
	return p.writeLine(resp.Body)
}

func (p mcpPump) failureMessage(err error) string {
	var he *cliclient.HTTPError
	if !errors.As(err, &he) {
		return "cannot reach the nexorious server: " + err.Error()
	}
	switch he.Status {
	case http.StatusUnauthorized:
		return fmt.Sprintf("invalid or expired API key for profile %q", p.profile)
	case http.StatusForbidden:
		if he.Message != "" {
			return he.Message
		}
		return "forbidden"
	case http.StatusNotFound:
		return "MCP is disabled on this server (MCP_ENABLED=false)"
	default:
		return he.Error()
	}
}

// writeLine writes one message as a single stdout line, compacting it if the
// server emitted it across several lines.
func (p mcpPump) writeLine(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.ContainsAny(b, "\r\n") {
		var buf bytes.Buffer
		if json.Compact(&buf, b) == nil {
			b = buf.Bytes()
		}
	}
	if _, err := p.out.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

func (p mcpPump) writeError(id json.RawMessage, text string) error {
	reply := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{JSONRPC: "2.0", ID: id}
	reply.Error.Code = jsonrpcServerError
	reply.Error.Message = text
	b, err := json.Marshal(reply)
	if err != nil {
		return fmt.Errorf("encode error reply: %w", err)
	}
	return p.writeLine(b)
}

// messageID peeks at a message's id. needsReply is false only for a
// well-formed notification (no id); an unparseable message is answered with a
// null id, as JSON-RPC prescribes for parse errors.
func messageID(msg []byte) (id json.RawMessage, needsReply bool) {
	var m struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return json.RawMessage("null"), true
	}
	if len(m.ID) == 0 {
		return nil, false
	}
	return m.ID, true
}

// sseData returns the data payload of each event in a text/event-stream body
// (multi-line data fields joined with "\n", per the SSE spec).
func sseData(body []byte) [][]byte {
	var events [][]byte
	var cur [][]byte
	flush := func() {
		if cur != nil {
			events = append(events, bytes.Join(cur, []byte("\n")))
			cur = nil
		}
	}
	for line := range bytes.Lines(body) {
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			flush()
			continue
		}
		if data, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			cur = append(cur, bytes.TrimPrefix(data, []byte(" ")))
		}
	}
	flush()
	return events
}
