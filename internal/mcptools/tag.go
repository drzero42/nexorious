package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/drzero42/nexorious/internal/cliclient"
	"github.com/drzero42/nexorious/internal/clientops"
)

type tagListOutput struct {
	Tags []cliclient.Tag `json:"tags"`
}

// tagBrief is the concise tag projection returned by write tools.
type tagBrief struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Color     *string `json:"color,omitempty"`
	GameCount int64   `json:"game_count"`
}

// tagWriteOutput is the standard output for tag write tools.
type tagWriteOutput struct {
	Tag     *tagBrief `json:"tag,omitempty"`
	Message string    `json:"message,omitempty"`
}

// tagCreateInput is the input schema for tag_create.
type tagCreateInput struct {
	Name  string  `json:"name" jsonschema:"tag name"`
	Color *string `json:"color,omitempty" jsonschema:"hex color e.g. #6B7280"`
}

// tagRenameInput is the input schema for tag_rename.
type tagRenameInput struct {
	Ref     string `json:"ref" jsonschema:"tag id (UUID) or name"`
	NewName string `json:"new_name" jsonschema:"new tag name"`
}

// tagRmInput is the input schema for tag_rm.
type tagRmInput struct {
	Ref string `json:"ref" jsonschema:"tag id (UUID) or name"`
}

func registerTagTools(s *toolSet, c *cliclient.Client, apiKey KeyFunc) {
	addTool(s, &mcp.Tool{Name: "tag_list", Description: "List all user tags with their game counts."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, tagListOutput, error) {
			key := apiKey(ctx)
			tags, err := c.ListTags(ctx, key)
			if err != nil {
				return nil, tagListOutput{}, toolError("tag_list", err)
			}
			return nil, tagListOutput{Tags: tags}, nil
		})

	addTool(s, &mcp.Tool{Name: "tag_create", Description: "Create a new tag."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in tagCreateInput) (*mcp.CallToolResult, tagWriteOutput, error) {
			key := apiKey(ctx)
			tag, err := c.CreateTag(ctx, key, in.Name, in.Color)
			if err != nil {
				return nil, tagWriteOutput{}, toolError("tag_create", err)
			}
			brief := &tagBrief{ID: tag.ID, Name: tag.Name, Color: tag.Color, GameCount: tag.GameCount}
			return nil, tagWriteOutput{Tag: brief, Message: fmt.Sprintf("Created tag %q (%s).", tag.Name, tag.ID)}, nil
		})

	addTool(s, &mcp.Tool{Name: "tag_rename", Description: "Rename a tag. Resolves by id (UUID) or name (case-insensitive)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in tagRenameInput) (*mcp.CallToolResult, tagWriteOutput, error) {
			key := apiKey(ctx)
			tag, err := clientops.ResolveTagRef(ctx, c, key, in.Ref)
			if err != nil {
				return nil, tagWriteOutput{}, toolError("tag_rename", err)
			}
			oldName := tag.Name
			updated, err := c.UpdateTag(ctx, key, tag.ID, &in.NewName, nil)
			if err != nil {
				return nil, tagWriteOutput{}, toolError("tag_rename", err)
			}
			brief := &tagBrief{ID: updated.ID, Name: updated.Name, Color: updated.Color, GameCount: updated.GameCount}
			return nil, tagWriteOutput{Tag: brief, Message: fmt.Sprintf("Renamed %q to %q.", oldName, updated.Name)}, nil
		})

	addTool(s, &mcp.Tool{Name: "tag_rm", Description: "Delete a tag. Resolves by id (UUID) or name (case-insensitive)."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in tagRmInput) (*mcp.CallToolResult, tagWriteOutput, error) {
			key := apiKey(ctx)
			tag, err := clientops.ResolveTagRef(ctx, c, key, in.Ref)
			if err != nil {
				return nil, tagWriteOutput{}, toolError("tag_rm", err)
			}
			if err := c.DeleteTag(ctx, key, tag.ID); err != nil {
				return nil, tagWriteOutput{}, toolError("tag_rm", err)
			}
			return nil, tagWriteOutput{Message: fmt.Sprintf("Deleted tag %q (%s).", tag.Name, tag.ID)}, nil
		})
}
