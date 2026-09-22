package tunnels

import (
	"context"
	"net/http"
	"strings"
)

const (
	ScopeAccountRead       = "account:read"
	ScopeTunnelsRead       = "tunnels:read"
	ScopeUsageRead         = "usage:read"
	ScopeTokensRead        = "tokens:read"
	ScopeBillingRead       = "billing:read"
	ScopeNotificationsRead = "notifications:read"
	ScopeRegistrarRead     = "registrar:read"
	ScopeCloudRead         = "cloud:read"

	// ScopeMCPConnect carries no authority. The consent screen adds it to
	// every token it mints.
	ScopeMCPConnect = "mcp:connect"

	ScopeWildcard = "*"
)

// explicitOnly lists the scopes a wildcard token does not satisfy. tBilling,
// tRegistrar and tServerless opened to CLI tokens after scopes existed, and
// they refuse "*". See tBilling/internal/handlers/scope.go.
var explicitOnly = map[string]bool{
	ScopeBillingRead:   true,
	ScopeRegistrarRead: true,
	ScopeCloudRead:     true,
}

func ExplicitOnly(scope string) bool { return explicitOnly[scope] }

type Self struct {
	Prefix    string   `json:"prefix"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	OwnerType string   `json:"owner_type"`
}

type Grants struct {
	Scopes     map[string]bool
	Discovered bool
	Name       string
	Prefix     string
}

func (g Grants) Has(scope string) bool {
	if !g.Discovered {
		return true
	}
	if g.Scopes[scope] {
		return true
	}
	return g.Scopes[ScopeWildcard] && !explicitOnly[scope]
}

// IsWildcard reports an all-access token that was not made on the consent
// screen.
func (g Grants) IsWildcard() bool {
	return g.Discovered && g.Scopes[ScopeWildcard] && !g.Scopes[ScopeMCPConnect]
}

func DiscoverSelf(ctx context.Context, c *Client) (Grants, error) {
	var self Self
	err := c.Get(ctx, "/api/v1/tokens/self", &self)
	if err != nil {
		var pe *Error
		if ok := asErr(err, &pe); ok && pe.Status == http.StatusNotFound {
			return Grants{Discovered: false}, nil
		}
		return Grants{}, err
	}
	set := make(map[string]bool, len(self.Scopes))
	for _, s := range self.Scopes {
		set[strings.TrimSpace(s)] = true
	}
	return Grants{
		Scopes:     set,
		Discovered: true,
		Name:       self.Name,
		Prefix:     self.Prefix,
	}, nil
}

func asErr(err error, target **Error) bool {
	if e, ok := err.(*Error); ok {
		*target = e
		return true
	}
	return false
}
