# MCP is served by the server; nexctl only relays it

MCP used to be hosted inside `nexctl`, so an agent could reach Nexorious only where `nexctl` was installed and configured. That ruled out agents running in containers, and an older `nexctl` could fall out of step with a newer server's API.

We decided that the server serves MCP at `POST /api/mcp` (#1257), and that `nexctl mcp serve` is a plain stdin↔HTTP pump for clients that only speak stdio (#1262). The pump does not parse the protocol beyond reading a message's `id`, so a server protocol upgrade never needs a `nexctl` upgrade. The MCP Go SDK is a server-only dependency: `nexctl` must not link it or `internal/mcptools`.

## Considered options

- **Keep MCP in `nexctl`** (the previous design). Rejected: it needs `nexctl` next to every agent, and a stale `nexctl` drifts from the API.
- **A protocol-aware bridge in `nexctl`.** Rejected: it would pin `nexctl` to an MCP protocol version, which is the coupling this decision removes.

## Consequences

- Changing a tool needs a server release, not a `nexctl` release.
- The operations shared by the CLI and the tools live in `internal/clientops`, which imports neither cobra nor the MCP SDK. CLI commands import `clientops`, never `mcptools`. `TestNexctlDoesNotLinkMCPSDK` guards the boundary.
- Tools still call the REST API through `cliclient`, over an in-process transport into the server's own router. Every sub-call reuses the caller's API key and is re-authenticated and re-scoped; there is no internal bypass.
- A 404 or 405 from `/api/mcp` means the server has no MCP endpoint (`MCP_ENABLED=false`, or too old). The pump reports it as a JSON-RPC error.
