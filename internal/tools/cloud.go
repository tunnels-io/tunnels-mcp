package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/tunnels-io/tunnels-mcp/internal/mcp"
	"github.com/tunnels-io/tunnels-mcp/internal/tunnels"
)

// CloudTool is a cloud app tool that takes an app. Path has one %s for the
// app id.
type CloudTool struct {
	Name        string
	Scope       string
	Method      string
	Path        string
	Description string
}

var CloudTools = []CloudTool{
	{
		Name:        "list_cloud_deployments",
		Scope:       tunnels.ScopeCloudRead,
		Method:      http.MethodGet,
		Path:        "/api/v1/cloud/apps/%s/deployments",
		Description: "Deploy history of one cloud app, newest first: commit, status, error and time.",
	},
	{
		Name:   "redeploy_cloud_app",
		Scope:  tunnels.ScopeCloudDeploy,
		Method: http.MethodPost,
		Path:   "/api/v1/cloud/apps/%s/redeploy",
		Description: "Start a fresh deploy of one cloud app: a new VM with a fresh copy of its source. " +
			"The app is offline for a few seconds. Returns at once; follow it with list_cloud_deployments.",
	},
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type appArgs struct {
	App string `json:"app"`
}

type cloudApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// resolveApp turns an id or an exact app name into an id. A name needs
// cloud:read; canList says whether the token has it.
func resolveApp(ctx context.Context, c *tunnels.Client, app string, canList bool) (string, error) {
	app = strings.TrimSpace(app)
	if app == "" {
		return "", errors.New("app is required. Give the app id or its exact name from list_cloud_apps.")
	}
	if uuidRe.MatchString(app) && !canList {
		return strings.ToLower(app), nil
	}
	if !canList {
		return "", errors.New("this token can not list cloud apps (cloud:read), so give the app id, not its name")
	}
	var page struct {
		Apps []cloudApp `json:"apps"`
	}
	if err := c.Get(ctx, "/api/v1/cloud/apps", &page); err != nil {
		return "", err
	}
	var names []string
	for _, a := range page.Apps {
		if strings.EqualFold(a.ID, app) || a.Name == app {
			return a.ID, nil
		}
		names = append(names, a.Name)
	}
	sort.Strings(names)
	have := "none"
	if len(names) > 0 {
		have = strings.Join(names, ", ")
	}
	return "", fmt.Errorf("no cloud app %q in this tunnels.io account. Apps here: %s. "+
		"An app of another tunnels.io account needs a token made in that account", app, have)
}

func (t CloudTool) Registration(c *tunnels.Client, canList bool) mcp.Registration {
	tool := mcp.Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"app": map[string]any{
					"type":        "string",
					"description": "The app id, or its exact name as list_cloud_apps shows it.",
				},
			},
			"required": []string{"app"},
		},
	}
	if t.Method != http.MethodGet {
		tool.Annotations = &mcp.ToolAnnotations{Title: "Redeploy a cloud app", OpenWorldHint: true}
	}
	return mcp.Registration{
		Tool: tool,
		Handler: func(ctx context.Context, raw json.RawMessage) *mcp.CallToolResult {
			var a appArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &a); err != nil {
					return mcp.Errorf("the arguments were not valid JSON: " + err.Error())
				}
			}
			id, err := resolveApp(ctx, c, a.App, canList)
			if err != nil {
				return mcp.Errorf(err.Error())
			}
			path := fmt.Sprintf(t.Path, url.PathEscape(id))
			var out json.RawMessage
			if t.Method == http.MethodGet {
				err = c.Get(ctx, path, &out)
			} else {
				err = c.Post(ctx, path, &out)
			}
			if err != nil {
				var pe *tunnels.Error
				if errors.As(err, &pe) && pe.Status == http.StatusNotFound {
					return mcp.Errorf("no cloud app " + id + " in this tunnels.io account, or this deployment " +
						"does not have " + t.Name + " yet")
				}
				return mcp.Errorf(err.Error())
			}
			pretty, perr := json.MarshalIndent(out, "", "  ")
			if perr != nil {
				return mcp.Errorf("tunnels.io returned a response this client could not format")
			}
			if t.Method != http.MethodGet {
				return mcp.Text("The redeploy is queued.\n\n" + string(pretty) +
					"\n\nCall list_cloud_deployments to see when it is success or failed.")
			}
			return mcp.Text(string(pretty))
		},
	}
}
