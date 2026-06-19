//go:build tools

// tools.go pins build-time dependencies that are not yet imported by production
// code. The "tools" build tag prevents this file from being compiled into the
// nexctl binary.
package main

import (
	_ "github.com/modelcontextprotocol/go-sdk/mcp"
)
