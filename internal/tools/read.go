package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tunnels-io/tunnels-mcp/internal/mcp"
	"github.com/tunnels-io/tunnels-mcp/internal/tunnels"
)

type ReadTool struct {
	Name        string
	Scope       string
	Path        string
	Description string
	Service     string
}

var ReadTools = []ReadTool{
	{
		Name:        "list_tunnels",
		Scope:       tunnels.ScopeTunnelsRead,
		Path:        "/api/v1/me/active-tunnels",
		Service:     "tEnforce",
		Description: "Tunnels running right now, with their public URLs and local addresses.",
	},
	{
		Name:        "get_usage",
		Scope:       tunnels.ScopeUsageRead,
		Path:        "/api/v1/usage/summary",
		Service:     "tEnforce",
		Description: "Bandwidth, requests and quota left for the current billing period.",
	},
	{
		Name:        "get_metrics",
		Scope:       tunnels.ScopeUsageRead,
		Path:        "/api/v1/metrics/current",
		Service:     "tEnforce",
		Description: "Live traffic right now. get_usage gives the billing-period totals instead.",
	},
	{
		Name:        "get_account",
		Scope:       tunnels.ScopeAccountRead,
		Path:        "/api/v1/auth/me",
		Service:     "tAuth",
		Description: "Email, name and current plan.",
	},
	{
		Name:        "list_tokens",
		Scope:       tunnels.ScopeTokensRead,
		Path:        "/api/v1/tokens",
		Service:     "tTokens",
		Description: "CLI tokens by name, prefix and permissions. Never the secrets.",
	},
	{
		Name:        "list_invoices",
		Scope:       tunnels.ScopeBillingRead,
		Path:        "/api/v1/invoices",
		Service:     "tBilling",
		Description: "Invoices with amounts, dates and payment status.",
	},
	{
		Name:        "list_domains",
		Scope:       tunnels.ScopeRegistrarRead,
		Path:        "/api/v1/purchased-domains",
		Service:     "tRegistrar",
		Description: "Domains bought through tunnels.io, with status and renewal dates.",
	},
	{
		Name:        "list_notifications",
		Scope:       tunnels.ScopeNotificationsRead,
		Path:        "/api/v1/me/notifications",
		Service:     "tNotify",
		Description: "In-app notifications, newest first.",
	},
	{
		Name:        "list_cloud_apps",
		Scope:       tunnels.ScopeCloudRead,
		Path:        "/api/v1/cloud/apps",
		Service:     "tServerless",
		Description: "Serverless apps, with status and hostnames.",
	},
}

func (t ReadTool) Registration(c *tunnels.Client) mcp.Registration {
	return mcp.Registration{
		Tool: mcp.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: mcp.EmptySchema(),
		},
		Handler: func(ctx context.Context, _ json.RawMessage) *mcp.CallToolResult {
			var raw json.RawMessage
			if err := c.Get(ctx, t.Path, &raw); err != nil {
				return mcp.Errorf(err.Error())
			}
			pretty, perr := json.MarshalIndent(raw, "", "  ")
			if perr != nil {
				return mcp.Errorf("tunnels.io returned a response this client could not format")
			}
			return mcp.Text(string(pretty))
		},
	}
}

func (t ReadTool) Describe() string {
	return fmt.Sprintf("%-20s %-20s %s", t.Name, t.Scope, t.Service)
}
