# tunnels-mcp

MCP server for [tunnels.io](https://tunnels.io).

It runs locally, holds one CLI token, and gives an assistant read access to the
account. It can also open a tunnel.

## Install

```
go install github.com/tunnels-io/tunnels-mcp/cmd/tunnels-mcp@v0.0.1
```

## Token

Create one at `tunnels.io/account/tokens`. Tick the permissions you want.

Skipping a permission is not a broken tool. The server reads the token's
permissions at startup and lists only the tools that work.

```
TUNNELS_TOKEN     required, starts tnl_
TUNNELS_API       default https://tunnels.io
TUNNELS_CLI       default "tunnels" on PATH
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

## Licence

MIT. See [LICENSE](LICENSE).
