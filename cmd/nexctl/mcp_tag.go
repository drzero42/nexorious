package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
)

type tagListOutput struct {
	Tags []cliclient.Tag `json:"tags"`
}

func registerTagTools(s *mcp.Server, c *cliclient.Client, key string) {
	mcp.AddTool(s, &mcp.Tool{Name: "tag_list", Description: "List all user tags with their game counts."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, tagListOutput, error) {
			tags, err := c.ListTags(key)
			if err != nil {
				return nil, tagListOutput{}, mcpToolError("tag_list", err)
			}
			return nil, tagListOutput{Tags: tags}, nil
		})
}
