package tools

import (
	"strings"
	"testing"

	"github.com/tunnels-io/tunnels-mcp/internal/tunnels"
)

var wantScope = map[string]string{
	"/api/v1/me/active-tunnels": tunnels.ScopeTunnelsRead,
	"/api/v1/usage/summary":     tunnels.ScopeUsageRead,
	"/api/v1/metrics/current":   tunnels.ScopeUsageRead,
	"/api/v1/auth/me":           tunnels.ScopeAccountRead,
	"/api/v1/tokens":            tunnels.ScopeTokensRead,
	"/api/v1/invoices":          tunnels.ScopeBillingRead,
	"/api/v1/purchased-domains": tunnels.ScopeRegistrarRead,
	"/api/v1/me/notifications":  tunnels.ScopeNotificationsRead,
	"/api/v1/cloud/apps":        tunnels.ScopeCloudRead,
}

func TestEveryToolDeclaresThePlatformScope(t *testing.T) {
	if len(ReadTools) != len(wantScope) {
		t.Fatalf("tools=%d routes=%d", len(ReadTools), len(wantScope))
	}
	for _, tool := range ReadTools {
		want, known := wantScope[tool.Path]
		if !known {
			t.Fatalf("%s: path %s not in wantScope", tool.Name, tool.Path)
		}
		if tool.Scope != want {
			t.Fatalf("%s: scope=%q want=%q (%s)", tool.Name, tool.Scope, want, tool.Path)
		}
	}
}

func TestEveryToolScopeIsARead(t *testing.T) {
	for _, tool := range ReadTools {
		if !strings.HasSuffix(tool.Scope, ":read") {
			t.Fatalf("%s: scope=%q not a read", tool.Name, tool.Scope)
		}
		if tool.Scope == tunnels.ScopeWildcard {
			t.Fatalf("%s: wildcard scope", tool.Name)
		}
	}
}

func TestToolNamesAreUniqueAndWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range ReadTools {
		if seen[tool.Name] {
			t.Fatalf("%s: duplicate", tool.Name)
		}
		seen[tool.Name] = true
		if tool.Name == "" || strings.ContainsAny(tool.Name, " /.") {
			t.Fatalf("bad name %q", tool.Name)
		}
	}
}

func TestDescriptionsAreUsable(t *testing.T) {
	for _, tool := range ReadTools {
		if len(tool.Description) < 25 {
			t.Fatalf("%s: description %d chars", tool.Name, len(tool.Description))
		}
	}
}

func TestPathsAreAbsoluteAPIPaths(t *testing.T) {
	for _, tool := range ReadTools {
		if !strings.HasPrefix(tool.Path, "/api/v1/") {
			t.Fatalf("%s: path=%q", tool.Name, tool.Path)
		}
	}
}
