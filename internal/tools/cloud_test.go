package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tunnels-io/tunnels-mcp/internal/mcp"
	"github.com/tunnels-io/tunnels-mcp/internal/tunnels"
)

const (
	testToken = "tnl_cloudtest_secret_value"
	appID     = "2473cb9b-d444-43eb-9ae0-9c0ec1c15225"
)

// The server-side gate of each route (tServerless server.go).
var wantCloud = map[string]struct{ scope, method string }{
	"/api/v1/cloud/apps/%s/deployments": {tunnels.ScopeCloudRead, http.MethodGet},
	"/api/v1/cloud/apps/%s/redeploy":    {tunnels.ScopeCloudDeploy, http.MethodPost},
}

func TestCloudToolsDeclareThePlatformScope(t *testing.T) {
	if len(CloudTools) != len(wantCloud) {
		t.Fatalf("tools=%d routes=%d", len(CloudTools), len(wantCloud))
	}
	for _, tool := range CloudTools {
		w, ok := wantCloud[tool.Path]
		if !ok {
			t.Fatalf("%s: path %s not pinned", tool.Name, tool.Path)
		}
		if tool.Scope != w.scope || tool.Method != w.method {
			t.Fatalf("%s: %s %s, want %s %s", tool.Name, tool.Method, tool.Scope, w.method, w.scope)
		}
	}
}

func TestCloudDeployIsExplicitOnly(t *testing.T) {
	if !tunnels.ExplicitOnly(tunnels.ScopeCloudDeploy) {
		t.Fatal("tServerless refuses * for cloud:deploy")
	}
	wild := tunnels.Grants{Discovered: true, Scopes: map[string]bool{"*": true}}
	if wild.Has(tunnels.ScopeCloudDeploy) {
		t.Fatal("a wildcard token must not get the redeploy tool")
	}
	read := tunnels.Grants{Discovered: true, Scopes: map[string]bool{tunnels.ScopeCloudRead: true}}
	if read.Has(tunnels.ScopeCloudDeploy) {
		t.Fatal("cloud:read must not get the redeploy tool")
	}
}

func TestToolNamesStayUnique(t *testing.T) {
	seen := map[string]bool{"start_tunnel": true, "stop_tunnel": true}
	for _, r := range ReadTools {
		seen[r.Name] = true
	}
	for _, c := range CloudTools {
		if seen[c.Name] {
			t.Fatalf("%s: duplicate", c.Name)
		}
		seen[c.Name] = true
	}
}

type hit struct{ method, path, auth string }

func fakeCloud(t *testing.T, status int, body string) (*tunnels.Client, *[]hit) {
	t.Helper()
	var hits []hit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path, r.Header.Get("Authorization")})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/cloud/apps" {
			_, _ = w.Write([]byte(`{"apps":[{"id":"` + appID + `","name":"curlybrackets.codes"},{"id":"a566b9a1-db96-4963-9cec-c2dc29c7c098","name":"golangDownload"}]}`))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return tunnels.New(srv.URL, testToken), &hits
}

func call(t *testing.T, reg mcp.Registration, args string) *mcp.CallToolResult {
	t.Helper()
	res := reg.Handler(context.Background(), json.RawMessage(args))
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), testToken) {
		t.Fatal("the token appeared in a tool result")
	}
	return res
}

func tool(name string) CloudTool {
	for _, c := range CloudTools {
		if c.Name == name {
			return c
		}
	}
	panic(name)
}

func TestRedeployByNamePostsToTheApp(t *testing.T) {
	c, hits := fakeCloud(t, http.StatusAccepted, `{"deployment":{"status":"pending"}}`)
	res := call(t, tool("redeploy_cloud_app").Registration(c, true), `{"app":"curlybrackets.codes"}`)
	if res.IsError {
		t.Fatalf("error: %s", res.Content[0].Text)
	}
	last := (*hits)[len(*hits)-1]
	if last.method != http.MethodPost || last.path != "/api/v1/cloud/apps/"+appID+"/redeploy" {
		t.Fatalf("called %s %s", last.method, last.path)
	}
	if last.auth != "Bearer "+testToken {
		t.Fatal("the bearer was not sent")
	}
	if !strings.Contains(res.Content[0].Text, "queued") || !strings.Contains(res.Content[0].Text, "list_cloud_deployments") {
		t.Fatalf("result: %s", res.Content[0].Text)
	}
}

func TestDeploymentsByIDWithoutReadSkipsTheList(t *testing.T) {
	c, hits := fakeCloud(t, http.StatusOK, `{"deployments":[]}`)
	res := call(t, tool("list_cloud_deployments").Registration(c, false), `{"app":"`+strings.ToUpper(appID)+`"}`)
	if res.IsError {
		t.Fatalf("error: %s", res.Content[0].Text)
	}
	if len(*hits) != 1 || (*hits)[0].method != http.MethodGet || (*hits)[0].path != "/api/v1/cloud/apps/"+appID+"/deployments" {
		t.Fatalf("calls: %+v", *hits)
	}
}

func TestUnknownAppNamesTheAppsAndTheOtherAccountCase(t *testing.T) {
	c, hits := fakeCloud(t, http.StatusOK, `{}`)
	res := call(t, tool("redeploy_cloud_app").Registration(c, true), `{"app":"nope"}`)
	if !res.IsError {
		t.Fatal("an unknown app must be an error")
	}
	msg := res.Content[0].Text
	for _, want := range []string{"curlybrackets.codes", "golangDownload", "another tunnels.io account"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in %q", want, msg)
		}
	}
	for _, h := range *hits {
		if h.method == http.MethodPost {
			t.Fatal("nothing may be posted for an unknown app")
		}
	}
}

func TestNameWithoutReadScopeAsksForTheID(t *testing.T) {
	c, hits := fakeCloud(t, http.StatusOK, `{}`)
	res := call(t, tool("redeploy_cloud_app").Registration(c, false), `{"app":"curlybrackets.codes"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "app id") || len(*hits) != 0 {
		t.Fatalf("res=%+v hits=%d", res, len(*hits))
	}
}

func TestMissingAppIsAnError(t *testing.T) {
	c, hits := fakeCloud(t, http.StatusOK, `{}`)
	for _, args := range []string{`{}`, `{"app":"  "}`, ``} {
		res := call(t, tool("redeploy_cloud_app").Registration(c, true), args)
		if !res.IsError || !strings.Contains(res.Content[0].Text, "app is required") {
			t.Fatalf("%q: %+v", args, res)
		}
	}
	if len(*hits) != 0 {
		t.Fatal("no call without an app")
	}
}

func TestRedeployErrorsReachTheAssistant(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusForbidden, `{"error":"This token does not have the cloud:deploy permission."}`, "cloud:deploy permission"},
		{http.StatusConflict, `{"error":"A deploy of this app is running now. Wait for it to finish, then try again."}`, "running now"},
		{http.StatusNotFound, `{"error":"app not found"}`, "no cloud app " + appID},
		{http.StatusInternalServerError, `{"error":"boom ` + testToken + `"}`, "server error"},
	} {
		c, hits := fakeCloud(t, tc.status, tc.body)
		res := call(t, tool("redeploy_cloud_app").Registration(c, false), `{"app":"`+appID+`"}`)
		if !res.IsError || !strings.Contains(res.Content[0].Text, tc.want) {
			t.Fatalf("%d: %+v", tc.status, res)
		}
		if len(*hits) != 1 {
			t.Fatalf("%d: a write must never be retried, got %d calls", tc.status, len(*hits))
		}
	}
}

func TestOnlyTheWriteToolHasAnnotations(t *testing.T) {
	c := tunnels.New("http://127.0.0.1:1", testToken)
	for _, r := range ReadTools {
		if r.Registration(c).Tool.Annotations != nil {
			t.Fatalf("%s: read tools keep their old tools/list entry", r.Name)
		}
	}
	if tool("list_cloud_deployments").Registration(c, true).Tool.Annotations != nil {
		t.Fatal("list_cloud_deployments is a read")
	}
	a := tool("redeploy_cloud_app").Registration(c, true).Tool.Annotations
	if a == nil || a.ReadOnlyHint || a.DestructiveHint || a.IdempotentHint {
		t.Fatalf("redeploy annotations = %+v", a)
	}
	b, _ := json.Marshal(tool("redeploy_cloud_app").Registration(c, true).Tool)
	if !strings.Contains(string(b), `"readOnlyHint":false`) {
		t.Fatalf("the hint must be sent explicitly: %s", b)
	}
}
