package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
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

func TestMCPConfigStanzaProfile(t *testing.T) {
	seedProfile(t, "https://example.test")
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"mcp", "config", "--profile", "work"})
	if err := root.Execute(); err != nil {
		t.Fatalf("mcp config --profile: %v\n%s", err, out.String())
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
	want := []string{"mcp", "serve", "--profile", "work"}
	if !ok || s.Command != "nexctl" || !slices.Equal(s.Args, want) {
		t.Fatalf("stanza args = %v, want %v", s.Args, want)
	}
}
