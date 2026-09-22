# tunnels-mcp

MCP server for [tunnels.io](https://tunnels.io).

It runs locally, holds one CLI token, and gives an assistant read access to the
account. It can also open a tunnel.

## Install

Download a binary for your platform from the
[releases page](https://github.com/tunnels-io/tunnels-mcp/releases), or build it:

```
go install github.com/tunnels-io/tunnels-mcp/cmd/tunnels-mcp@v0.0.2
```

Clients that take MCP bundles, such as Claude Desktop, can install
`tunnels-mcp-<version>.mcpb` from the same page. It asks for the token.

The server is listed in the MCP Registry as `io.github.tunnels-io/tunnels-mcp`.

## Token

Create one at `tunnels.io/account/tokens`. Tick the permissions you want.

Skipping a permission is not a broken tool. The server reads the token's
permissions at startup and lists only the tools that work.

An older all-access token (`*`) works for everything except billing, domains
and cloud apps. Those three services refuse `*`, so the server does not list
their tools for such a token. Make a token on the page above to reach them.

```
TUNNELS_TOKEN     required, starts tnl_
TUNNELS_API       default https://tunnels.io
TUNNELS_CLI       default "tunnels" on PATH
TUNNELS_INSPECT   unset. The CLI inspector is off; set an address to enable it.
```

A config file works too, at `~/.config/tunnels/mcp.json`:

```json
{ "token": "tnl_..." }
```

The environment wins.

## Client setup

```json
{
  "mcpServers": {
    "tunnels": {
      "command": "tunnels-mcp",
      "env": { "TUNNELS_TOKEN": "tnl_..." }
    }
  }
}
```

## Tools

| Tool | Permission |
|---|---|
| `list_tunnels` | `tunnels:read` |
| `get_usage` | `usage:read` |
| `get_metrics` | `usage:read` |
| `get_account` | `account:read` |
| `list_tokens` | `tokens:read` |
| `list_invoices` | `billing:read` |
| `list_domains` | `registrar:read` |
| `list_notifications` | `notifications:read` |
| `list_cloud_apps` | `cloud:read` |
| `start_tunnel` | none |
| `stop_tunnel` | none |

Reads only, apart from the last two. Those shell out to the `tunnels` CLI, which
must be on PATH. No tool changes a plan, buys a domain or deploys anything.

`start_tunnel` takes `http` (the default, served as HTTPS on the public URL) or
`tcp`.

## Token handling

The token goes to tunnels.io in a Bearer header, and to the CLI through the
environment. Never a command line. Never a log. Never a tool result.

Error bodies are redacted before they are shown.

The server contacts tunnels.io and nothing else.

## Build

```
make check
make build
```

No dependencies.

## Release

Push a tag such as `v0.0.2`. The `release` workflow builds six binaries and
the MCPB bundle, creates the GitHub release, and publishes `server.json` to the
MCP Registry. To build the same files locally, on macOS:

```
make release V=0.0.2
```

## Licence

MIT. See [LICENSE](LICENSE).
