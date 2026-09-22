package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const OpenTimeout = 45 * time.Second

type Tunnel struct {
	PublicURL string
	LocalAddr string
	Protocol  string

	cmd  *exec.Cmd
	once sync.Once
}

func (t *Tunnel) Stop() {
	t.once.Do(func() {
		if t.cmd != nil && t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
			_ = t.cmd.Wait()
		}
	})
}

type Runner struct {
	Bin   string
	Token string

	mu      sync.Mutex
	running map[string]*Tunnel
}

func NewRunner(bin, token string) *Runner {
	if strings.TrimSpace(bin) == "" {
		bin = "tunnels"
	}
	return &Runner{Bin: bin, Token: token, running: map[string]*Tunnel{}}
}

var ErrNoCLI = errors.New("the tunnels CLI was not found")

// InspectEnv turns the CLI's request inspector back on. The value goes
// straight to --inspect-addr, so "127.0.0.1:0" or a fixed address both work.
const InspectEnv = "TUNNELS_INSPECT"

// buildArgs builds the CLI argument list.
//
// The inspector is off by default. It has no auth, and a tunnel opened by an
// assistant is unattended, so leaving it on would expose the tunnel's headers
// to anything on the machine. Set InspectEnv to get it back.
//
// Flags go before the port. The CLI treats everything after it as pass-through.
func buildArgs(port int, subdomain, protocol string) []string {
	args := []string{"--events", "ndjson"}

	inspect := strings.TrimSpace(os.Getenv(InspectEnv))
	if inspect == "" {
		inspect = "disabled"
	}
	args = append(args, "--inspect-addr", inspect)

	if subdomain != "" {
		args = append(args, "--subdomain", subdomain)
	}
	return append(args, protocol, fmt.Sprint(port))
}

// forwarded is what the CLI reads, minus the token. Keep it an allowlist: an
// MCP server inherits the AI client's environment and should not hand all of it
// to a child.
//
// http_proxy is lowercase on purpose. The CLI reads no other spelling, so
// HTTPS_PROXY and HTTP_PROXY would be dead weight.
var forwarded = []string{
	"PATH",
	"HOME",
	"http_proxy",
	"TUNELS_API_BASE_URL",
	"TUNELS_NO_TELEMETRY",
	"NO_COLOR",
	"TZ",
	"HOSTNAME",
}

// noUpdateCheck stops the CLI's daily release check. It costs a 2s fetch and a
// stderr banner in a subprocess that cannot act on the result.
const noUpdateCheck = "TUNELS_NO_UPDATE_CHECK"

// buildEnv builds the child environment. It REPLACES the parent, so anything
// missing here does not reach the CLI.
func buildEnv(token string) []string {
	env := make([]string, 0, len(forwarded)+2)
	for _, k := range forwarded {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}

	// An explicit parent setting wins, so the check can be re-enabled.
	if v, ok := os.LookupEnv(noUpdateCheck); ok {
		env = append(env, noUpdateCheck+"="+v)
	} else {
		env = append(env, noUpdateCheck+"=1")
	}

	// Last and unconditional: a stray parent value must not win.
	return append(env, "TUNELS_AUTHTOKEN="+token)
}

func (r *Runner) Available() error {
	if _, err := exec.LookPath(r.Bin); err != nil {
		return fmt.Errorf("%w: %q is not on PATH. Install it from tunnels.io, or set TUNNELS_CLI to its path", ErrNoCLI, r.Bin)
	}
	return nil
}

func (r *Runner) Start(ctx context.Context, port int, subdomain, protocol string) (*Tunnel, error) {
	protocol, err := validate(port, protocol)
	if err != nil {
		return nil, err
	}
	if err := r.Available(); err != nil {
		return nil, err
	}

	cmd := exec.Command(r.Bin, buildArgs(port, subdomain, protocol)...)
	cmd.Env = buildEnv(r.Token)
	cmd.Stdin = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("could not read the CLI's output: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("could not start the tunnels CLI: %w", err)
	}

	type result struct {
		t   *Tunnel
		err error
	}
	done := make(chan result, 1)

	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var lastProblem string
		lost := false
		for sc.Scan() {
			ev, ok := ParseEvent(sc.Bytes())
			if !ok {
				continue
			}
			switch ev.Type {
			case EvTunnelOpened:
				t := &Tunnel{
					PublicURL: ev.PublicURL,
					LocalAddr: ev.LocalAddr,
					Protocol:  ev.Protocol,
					cmd:       cmd,
				}
				if lost {
					fmt.Fprintln(os.Stderr, "tunnels-mcp: the CLI event stream lost events before the tunnel opened")
				}
				done <- result{t: t}
				return
			case EvDropped:
				lost = true
			case EvError:
				lastProblem = ev.Message
			case EvDisconnected:
				lastProblem = ev.ErrorText
			case EvExiting:
				if lastProblem == "" {
					lastProblem = ev.Reason
				}
				done <- result{err: cliProblem(lastProblem)}
				return
			}
		}
		// Scan() stops on a clean EOF and on an error. Only the first means the
		// CLI exited; the second means this reader died while it was still up.
		if err := sc.Err(); err != nil {
			done <- result{err: streamProblem(lastProblem, err)}
			return
		}
		done <- result{err: cliProblem(lastProblem)}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, res.err
		}
		r.mu.Lock()
		r.running[res.t.PublicURL] = res.t
		r.mu.Unlock()
		return res.t, nil
	case <-time.After(OpenTimeout):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("the tunnel did not open within %s. Check that the local port is listening and that the token is valid", OpenTimeout)
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, ctx.Err()
	}
}

func (r *Runner) Stop(publicURL string) error {
	r.mu.Lock()
	t, ok := r.running[publicURL]
	delete(r.running, publicURL)
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("no tunnel is running at %s", publicURL)
	}
	t.Stop()
	return nil
}

func (r *Runner) List() []*Tunnel {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Tunnel, 0, len(r.running))
	for _, t := range r.running {
		out = append(out, t)
	}
	return out
}

func (r *Runner) StopAll() {
	r.mu.Lock()
	all := make([]*Tunnel, 0, len(r.running))
	for k, t := range r.running {
		all = append(all, t)
		delete(r.running, k)
	}
	r.mu.Unlock()
	for _, t := range all {
		t.Stop()
	}
}

// validate checks the arguments before anything is spawned, so a bad request
// is reported as itself rather than as a missing CLI.
func validate(port int, protocol string) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d is not a valid TCP port", port)
	}
	if protocol == "" {
		protocol = "http"
	}
	if protocol != "http" && protocol != "tcp" {
		return "", fmt.Errorf("protocol %q is not supported; use http or tcp", protocol)
	}
	return protocol, nil
}

func cliProblem(detail string) error {
	if strings.TrimSpace(detail) == "" {
		return errors.New("the tunnels CLI exited before the tunnel opened")
	}
	return fmt.Errorf("the tunnel could not be opened: %s", detail)
}

// streamProblem says the reader died, not the CLI. Keeps whatever the CLI
// managed to say first.
func streamProblem(detail string, err error) error {
	if strings.TrimSpace(detail) == "" {
		return fmt.Errorf("lost the tunnels CLI event stream before the tunnel opened: %w", err)
	}
	return fmt.Errorf("lost the tunnels CLI event stream after %q: %w", detail, err)
}
