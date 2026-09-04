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

func (r *Runner) Available() error {
	if _, err := exec.LookPath(r.Bin); err != nil {
		return fmt.Errorf("%w: %q is not on PATH. Install it from tunnels.io, or set TUNNELS_CLI to its path", ErrNoCLI, r.Bin)
	}
	return nil
}

func (r *Runner) Start(ctx context.Context, port int, subdomain, protocol string) (*Tunnel, error) {
	if err := r.Available(); err != nil {
		return nil, err
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port %d is not a valid TCP port", port)
	}
	if protocol == "" {
		protocol = "http"
	}
	if protocol != "http" && protocol != "tcp" && protocol != "tls" {
		return nil, fmt.Errorf("protocol %q is not supported; use http, tcp or tls", protocol)
	}

	args := []string{"--events", "ndjson"}
	if subdomain != "" {
		args = append(args, "--subdomain", subdomain)
	}
	args = append(args, protocol, fmt.Sprint(port))

	cmd := exec.Command(r.Bin, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TUNELS_AUTHTOKEN=" + r.Token,
	}
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

func cliProblem(detail string) error {
	if strings.TrimSpace(detail) == "" {
		return errors.New("the tunnels CLI exited before the tunnel opened")
	}
	return fmt.Errorf("the tunnel could not be opened: %s", detail)
}
