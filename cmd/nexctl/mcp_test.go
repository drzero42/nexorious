package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/clicfg"
)

func TestMCPConfigStanza(t *testing.T) {
	seedProfile(t, "https://example.test")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"mcp", "config"})
	if err := root.Execute(); err != nil {
		t.Fatalf("mcp config: %v\n%s", err, out.String())
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(out.Bytes(), &cfg); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	s, ok := cfg.MCPServers["nexorious"]
	if !ok || s.Command != "nexctl" || len(s.Args) != 2 || s.Args[0] != "mcp" || s.Args[1] != "serve" {
		t.Fatalf("stanza = %+v", cfg)
	}
}

// mcpSession spins up buildMCPServer pointed at restURL and returns a connected
// in-memory client session. Used by tool tests in tasks 4–7.
func mcpSession(t *testing.T, restURL string) *mcp.ClientSession { //nolint:unused // called by tool tests in tasks 4–7
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := buildMCPServer(clicfg.Profile{URL: restURL, Key: "test-key"})
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}
