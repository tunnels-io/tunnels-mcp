package tunnels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const probeToken = "tnl_ThisIsATestTokenValueThatMustNeverLeak"

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		contains string
	}{
		{
			name:     "403 passes the platform's own words through",
			status:   403,
			body:     `{"error":"This token does not have the billing:read permission."}`,
			contains: "billing:read",
		},
		{
			name:     "403 with no body still says something useful",
			status:   403,
			body:     `{}`,
			contains: "permission",
		},
		{
			name:     "401 names the fix",
			status:   401,
			body:     `{"error":"Invalid token"}`,
			contains: "tunnels.io/account/tokens",
		},
		{
			name:     "429 says to wait",
			status:   429,
			body:     `{}`,
			contains: "rate-limiting",
		},
		{
			name:     "404 says the endpoint is absent",
			status:   404,
			body:     `{}`,
			contains: "not available",
		},
		{
			name:     "500 says to retry",
			status:   500,
			body:     `{}`,
			contains: "server error",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			err := New(srv.URL, probeToken).Get(context.Background(), "/x", nil)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), c.contains) {
				t.Fatalf("msg=%q want substring %q", err.Error(), c.contains)
			}
		})
	}
}

func TestRegistrarErrorCodeIsKept(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"nope","error_code":"role.insufficient"}`))
	}))
	defer srv.Close()
	err := New(srv.URL, probeToken).Get(context.Background(), "/x", nil)
	pe, ok := err.(*Error)
	if !ok {
		t.Fatalf("type=%T", err)
	}
	if pe.ErrorCode != "role.insufficient" {
		t.Fatalf("error_code=%q", pe.ErrorCode)
	}
}

func TestTokenNeverAppearsInAnyError(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 502} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"saw ` + probeToken + `"}`))
		}))
		err := New(srv.URL, probeToken).Get(context.Background(), "/x", nil)
		srv.Close()
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), probeToken) {
			t.Fatalf("status=%d leaked: %q", status, err.Error())
		}
	}

	err := New("http://127.0.0.1:1", probeToken).Get(context.Background(), "/x?probe="+probeToken, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), probeToken) {
		t.Fatalf("leaked: %q", err.Error())
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	var out map[string]any
	err = New(srv.URL, probeToken).Get(context.Background(), "/x", &out)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), probeToken) {
		t.Fatalf("leaked: %q", err.Error())
	}
}

func TestRequestHeaders(t *testing.T) {
	var gotAuth, gotUA, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var out map[string]any
	if err := New(srv.URL, probeToken).Get(context.Background(), "/x", &out); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+probeToken {
		t.Fatalf("auth=%q", gotAuth)
	}
	if !strings.HasPrefix(gotUA, "tunnels-mcp/") {
		t.Fatalf("ua=%q", gotUA)
	}
	if strings.Contains(gotQuery, probeToken) {
		t.Fatal("token in query")
	}
}

func TestDiscoveryDegradesOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	g, err := DiscoverSelf(context.Background(), New(srv.URL, probeToken))
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if g.Discovered {
		t.Fatal("Discovered=true want false")
	}
	if !g.Has(ScopeBillingRead) {
		t.Fatal("Has=false want true")
	}
}

func TestDiscoveryFailsOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()
	if _, err := DiscoverSelf(context.Background(), New(srv.URL, probeToken)); err == nil {
		t.Fatal("want error")
	}
}

func TestDiscoveryReadsScopes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tokens/self" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Self{
			Prefix: "tnl_abc...", Name: "laptop", OwnerType: "user",
			Scopes: []string{ScopeMCPConnect, ScopeUsageRead},
		})
	}))
	defer srv.Close()
	g, err := DiscoverSelf(context.Background(), New(srv.URL, probeToken))
	if err != nil {
		t.Fatal(err)
	}
	if !g.Discovered || !g.Has(ScopeUsageRead) {
		t.Fatal("granted scope not held")
	}
	if g.Has(ScopeBillingRead) {
		t.Fatal("ungranted scope held")
	}
}

func TestWildcardSatisfiesEverything(t *testing.T) {
	g := Grants{Discovered: true, Scopes: map[string]bool{ScopeWildcard: true}}
	for _, s := range []string{ScopeUsageRead, ScopeBillingRead, ScopeCloudRead} {
		if !g.Has(s) {
			t.Fatalf("wildcard failed %s", s)
		}
	}
}
