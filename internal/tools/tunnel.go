package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tunnels-io/tunnels-mcp/internal/cli"
	"github.com/tunnels-io/tunnels-mcp/internal/mcp"
)

type startArgs struct {
	Port      int    `json:"port"`
	Subdomain string `json:"subdomain,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

func StartTunnel(r *cli.Runner) mcp.Registration {
	return mcp.Registration{
		Tool: mcp.Tool{
			Name: "start_tunnel",
			Description: "Expose a local port through tunnels.io and return the public URL. " +
				"Stays up until stop_tunnel or the end of the session.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"port": map[string]any{
						"type":        "integer",
						"description": "The local TCP port to expose, for example 3000.",
						"minimum":     1,
						"maximum":     65535,
					},
					"subdomain": map[string]any{
						"type":        "string",
						"description": "Optional. Ask for a specific subdomain instead of a generated one.",
					},
					"protocol": map[string]any{
						"type": "string",
						// The CLI takes only these two. It has no tls mode: "tls 3000"
						// fails with "invalid address 'tls': missing port".
						"enum":        []string{"http", "tcp"},
						"description": "Optional. http serves HTTPS on the public URL; tcp forwards raw TCP. Defaults to http.",
					},
				},
				"required": []string{"port"},
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) *mcp.CallToolResult {
			var a startArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &a); err != nil {
					return mcp.Errorf("the arguments were not valid JSON: " + err.Error())
				}
			}
			if a.Port == 0 {
				return mcp.Errorf("port is required. Give the local TCP port to expose, for example 3000.")
			}
			t, err := r.Start(ctx, a.Port, a.Subdomain, a.Protocol)
			if err != nil {
				return mcp.Errorf(err.Error())
			}
			return mcp.Text(fmt.Sprintf(
				"The tunnel is live.\n\npublic URL: %s\nforwarding to: %s\nprotocol: %s",
				t.PublicURL, t.LocalAddr, t.Protocol))
		},
	}
}

type stopArgs struct {
	PublicURL string `json:"public_url"`
}

func StopTunnel(r *cli.Runner) mcp.Registration {
	return mcp.Registration{
		Tool: mcp.Tool{
			Name:        "stop_tunnel",
			Description: "Close a tunnel opened by start_tunnel in this session.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"public_url": map[string]any{
						"type":        "string",
						"description": "The public URL that start_tunnel returned.",
					},
				},
				"required": []string{"public_url"},
			},
		},
		Handler: func(_ context.Context, raw json.RawMessage) *mcp.CallToolResult {
			var a stopArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &a); err != nil {
					return mcp.Errorf("the arguments were not valid JSON: " + err.Error())
				}
			}
			if a.PublicURL == "" {
				return mcp.Errorf("public_url is required. Use the URL that start_tunnel returned.")
			}
			if err := r.Stop(a.PublicURL); err != nil {
				return mcp.Errorf(err.Error())
			}
			return mcp.Text("Closed " + a.PublicURL)
		},
	}
}
