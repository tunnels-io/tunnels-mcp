package tunnels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// version is set at release time:
//
//	-ldflags "-X github.com/tunnels-io/tunnels-mcp/internal/tunnels.version=0.0.2"
var version string

// Version comes from the release, never from a hand-edited constant. It used
// to be "0.1.0" while the published tag was v0.0.1.
var Version = resolveVersion(version, buildVersion())

func buildVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return bi.Main.Version
}

// resolveVersion prefers the linker value, then the module version that
// `go install ...@v0.0.1` records, then a dev marker.
func resolveVersion(linked, module string) string {
	if v := strings.TrimSpace(linked); v != "" {
		return strings.TrimPrefix(v, "v")
	}
	if module != "" && module != "(devel)" {
		return strings.TrimPrefix(module, "v")
	}
	return "0.0.0-dev"
}

const TokenPrefix = "tnl_"

const TeamTokenPrefix = "tnlt_"

type Error struct {
	Status    int
	Message   string
	ErrorCode string
}

func (e *Error) Error() string { return e.Message }

func (e *Error) IsUnauthorized() bool { return e.Status == http.StatusUnauthorized }

func (e *Error) IsForbidden() bool { return e.Status == http.StatusForbidden }

type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func New(base, token string) *Client {
	return &Client{
		Base:  strings.TrimRight(base, "/"),
		Token: token,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) redact(b []byte) []byte {
	if c.Token == "" {
		return b
	}
	return []byte(strings.ReplaceAll(string(b), c.Token, "[redacted]"))
}

func (c *Client) Get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return &Error{Message: "could not build the request: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tunnels-mcp/"+Version)
	req.Header.Set("X-Tunnels-Client", "tunnels-mcp/"+Version)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return &Error{Message: "could not reach tunnels.io: " + netReason(err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 400 {
		return mapError(resp.StatusCode, c.redact(body))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &Error{
			Status:  resp.StatusCode,
			Message: "tunnels.io returned a response this client does not understand. The server may be a different version.",
		}
	}
	return nil
}

func mapError(status int, body []byte) *Error {
	var payload struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(body, &payload)
	msg := strings.TrimSpace(payload.Error)

	e := &Error{Status: status, ErrorCode: payload.ErrorCode}
	switch status {
	case http.StatusUnauthorized:
		e.Message = "The tunnels.io token is invalid or revoked. Create a new one at tunnels.io/account/tokens."
	case http.StatusForbidden:
		if msg == "" {
			msg = "This token does not have permission for that."
		}
		e.Message = msg
	case http.StatusTooManyRequests:
		e.Message = "tunnels.io is rate-limiting this token. Wait a moment and try again."
	case http.StatusNotFound:
		e.Message = "That endpoint is not available on this tunnels.io deployment."
	default:
		if status >= 500 {
			e.Message = "tunnels.io returned a server error. Try again shortly."
		} else if msg != "" {
			e.Message = msg
		} else {
			e.Message = fmt.Sprintf("tunnels.io returned HTTP %d.", status)
		}
	}
	return e
}

func netReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "the request timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "the request was cancelled"
	}
	return "the connection failed"
}
