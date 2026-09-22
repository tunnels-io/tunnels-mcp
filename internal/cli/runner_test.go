package cli

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestTunnelOpenedCarriesTheURL(t *testing.T) {
	line := `{"seq":4,"ts":"2025-01-01T00:00:00Z","type":"tunnel_opened","public_url":"https://abc.tunnels.io","local_addr":"127.0.0.1:3000","protocol":"http"}`
	ev, ok := ParseEvent([]byte(line))
	if !ok {
		t.Fatal("parse failed")
	}
	if ev.Type != EvTunnelOpened {
		t.Fatalf("type=%q", ev.Type)
	}
	if ev.PublicURL != "https://abc.tunnels.io" {
		t.Fatalf("public_url=%q", ev.PublicURL)
	}
	if ev.LocalAddr != "127.0.0.1:3000" || ev.Protocol != "http" {
		t.Fatalf("local_addr=%q protocol=%q", ev.LocalAddr, ev.Protocol)
	}
}

func TestDroppedCarriesTheCount(t *testing.T) {
	ev, ok := ParseEvent([]byte(`{"seq":9,"ts":"t","type":"dropped","count":17}`))
	if !ok || ev.Type != EvDropped {
		t.Fatal("parse failed")
	}
	if ev.Count != 17 {
		t.Fatalf("count=%d want=17", ev.Count)
	}
}

func TestFailureEventsKeepTheirReason(t *testing.T) {
	cases := []struct{ line, typ, want string }{
		{`{"seq":1,"ts":"t","type":"error","message":"bad token"}`, EvError, "bad token"},
		{`{"seq":2,"ts":"t","type":"disconnected","error":"connection refused"}`, EvDisconnected, "connection refused"},
		{`{"seq":3,"ts":"t","type":"exiting","reason":"signal"}`, EvExiting, "signal"},
	}
	for _, c := range cases {
		ev, ok := ParseEvent([]byte(c.line))
		if !ok || ev.Type != c.typ {
			t.Fatalf("%s: got %+v", c.typ, ev)
		}
		got := ev.Message + ev.ErrorText + ev.Reason
		if !strings.Contains(got, c.want) {
			t.Fatalf("%s: reason=%q", c.typ, got)
		}
	}
}

func TestUnrecognisedLinesAreSkipped(t *testing.T) {
	for _, line := range []string{"", "not json", `{"no":"type"}`, `[]`} {
		if _, ok := ParseEvent([]byte(line)); ok {
			t.Fatalf("parsed %q", line)
		}
	}
}

func TestUnknownEventTypeStillParses(t *testing.T) {
	ev, ok := ParseEvent([]byte(`{"seq":1,"ts":"t","type":"some_future_event","x":1}`))
	if !ok {
		t.Fatal("parse failed")
	}
	if ev.Type != "some_future_event" {
		t.Fatalf("type=%q", ev.Type)
	}
}

func TestAvailableNamesTheFix(t *testing.T) {
	err := NewRunner("definitely-not-a-real-binary-xyz", "tnl_x").Available()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "PATH") || !strings.Contains(err.Error(), "TUNNELS_CLI") {
		t.Fatalf("msg=%q", err.Error())
	}
}

func TestDefaultBinaryName(t *testing.T) {
	if got := NewRunner("", "tnl_x").Bin; got != "tunnels" {
		t.Fatalf("bin=%q want=tunnels", got)
	}
}

func TestTLSIsRefusedBeforeSpawning(t *testing.T) {
	// The CLI has no tls mode. The binary here does not exist, so if validation
	// ran after the CLI lookup the error would wrongly blame the CLI.
	_, err := NewRunner("definitely-not-a-real-binary-xyz", "tnl_x").Start(context.Background(), 3000, "", "tls")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "tls") || strings.Contains(err.Error(), "PATH") {
		t.Fatalf("msg=%q", err.Error())
	}
}

func TestValidateDefaultsToHTTP(t *testing.T) {
	p, err := validate(3000, "")
	if err != nil || p != "http" {
		t.Fatalf("p=%q err=%v", p, err)
	}
	if _, err := validate(3000, "tcp"); err != nil {
		t.Fatalf("tcp refused: %v", err)
	}
}

func TestValidateRefusesBadPorts(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		if _, err := validate(port, "http"); err == nil {
			t.Fatalf("port %d accepted", port)
		}
	}
}

// spawn: arguments and environment.

func argIndex(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

func envValue(env []string, key string) (string, bool) {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return strings.TrimPrefix(e, key+"="), true
		}
	}
	return "", false
}

// The inspector has no auth, so it stays off unless asked for.
func TestArgsDisableTheInspectorByDefault(t *testing.T) {
	t.Setenv(InspectEnv, "")
	args := buildArgs(3000, "", "http")
	i := argIndex(args, "--inspect-addr")
	if i < 0 {
		t.Fatalf("--inspect-addr missing from %v", args)
	}
	if args[i+1] != "disabled" {
		t.Errorf("--inspect-addr = %q, want \"disabled\"", args[i+1])
	}
}

func TestInspectOverrideIsPassedThrough(t *testing.T) {
	t.Setenv(InspectEnv, "127.0.0.1:0")
	args := buildArgs(3000, "", "http")
	i := argIndex(args, "--inspect-addr")
	if i < 0 || args[i+1] != "127.0.0.1:0" {
		t.Fatalf("override not honoured: %v", args)
	}
}

// The CLI passes through anything after the port, so flags must precede it.
func TestFlagsComeBeforeThePositionalPort(t *testing.T) {
	t.Setenv(InspectEnv, "")
	args := buildArgs(8080, "demo", "tcp")
	proto := argIndex(args, "tcp")
	if proto < 0 {
		t.Fatalf("protocol missing from %v", args)
	}
	for _, f := range []string{"--events", "--inspect-addr", "--subdomain"} {
		if i := argIndex(args, f); i < 0 || i > proto {
			t.Errorf("%s at %d, protocol at %d: flags must come first (%v)", f, i, proto, args)
		}
	}
	if args[len(args)-1] != "8080" {
		t.Errorf("port must be last, got %v", args)
	}
}

func TestSubdomainIsOmittedWhenEmpty(t *testing.T) {
	t.Setenv(InspectEnv, "")
	if i := argIndex(buildArgs(3000, "", "http"), "--subdomain"); i >= 0 {
		t.Error("--subdomain must not appear when no subdomain was asked for")
	}
}

// The CLI reads lowercase http_proxy. Dropping it broke proxied users.
func TestEnvForwardsHTTPProxy(t *testing.T) {
	t.Setenv("http_proxy", "http://proxy.corp:8080")
	if v, ok := envValue(buildEnv("tnl_x"), "http_proxy"); !ok || v != "http://proxy.corp:8080" {
		t.Errorf("http_proxy = %q ok=%v, want it forwarded", v, ok)
	}
}

func TestEnvDoesNotForwardUppercaseProxy(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy.corp:8080")
	t.Setenv("HTTP_PROXY", "http://proxy.corp:8080")
	env := buildEnv("tnl_x")
	for _, k := range []string{"HTTPS_PROXY", "HTTP_PROXY"} {
		if _, ok := envValue(env, k); ok {
			t.Errorf("%s was forwarded; the CLI cannot read it", k)
		}
	}
}

func TestEnvSuppressesTheUpdateCheck(t *testing.T) {
	os.Unsetenv(noUpdateCheck)
	if v, ok := envValue(buildEnv("tnl_x"), noUpdateCheck); !ok || v != "1" {
		t.Errorf("%s = %q ok=%v, want \"1\"", noUpdateCheck, v, ok)
	}
}

func TestEnvKeepsAnExplicitUpdateCheckSetting(t *testing.T) {
	t.Setenv(noUpdateCheck, "0")
	if v, _ := envValue(buildEnv("tnl_x"), noUpdateCheck); v != "0" {
		t.Errorf("%s = %q, want the parent value \"0\" to survive", noUpdateCheck, v)
	}
}

func TestEnvTokenWinsOverTheParent(t *testing.T) {
	t.Setenv("TUNELS_AUTHTOKEN", "tnl_from_the_parent")
	if v, _ := envValue(buildEnv("tnl_configured"), "TUNELS_AUTHTOKEN"); v != "tnl_configured" {
		t.Errorf("TUNELS_AUTHTOKEN = %q, want the configured token to win", v)
	}
}

// The allowlist is a security property. Do not replace it with os.Environ().
func TestEnvDoesNotLeakUnlistedVariables(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "should-not-reach-the-child")
	t.Setenv("GITHUB_TOKEN", "should-not-reach-the-child")
	env := buildEnv("tnl_x")
	for _, e := range env {
		if strings.Contains(e, "should-not-reach-the-child") {
			t.Fatalf("an unlisted variable reached the child: %q", e)
		}
	}
}

// Scan() stops on EOF and on error; only one means the CLI exited.

func TestStreamProblemNamesTheCause(t *testing.T) {
	err := streamProblem("", bufio.ErrTooLong)
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("the cause must be wrapped, got %v", err)
	}
	if strings.Contains(err.Error(), "exited") {
		t.Errorf("must not claim the CLI exited: %q", err)
	}
	if !strings.Contains(err.Error(), "lost") {
		t.Errorf("must say the stream was lost: %q", err)
	}
}

func TestStreamProblemKeepsWhatTheCLISaid(t *testing.T) {
	err := streamProblem("subdomain already taken", bufio.ErrTooLong)
	if !strings.Contains(err.Error(), "subdomain already taken") {
		t.Errorf("the CLI's own message must survive: %q", err)
	}
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Errorf("the cause must still be wrapped: %v", err)
	}
}

// Only the error path moved; this pins the old message.
func TestCleanEndOfStreamKeepsItsMessage(t *testing.T) {
	if got := cliProblem("").Error(); got != "the tunnels CLI exited before the tunnel opened" {
		t.Errorf("clean-EOF message changed: %q", got)
	}
	if got := cliProblem("no such port").Error(); got != "the tunnel could not be opened: no such port" {
		t.Errorf("detailed message changed: %q", got)
	}
}
