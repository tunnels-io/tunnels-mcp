package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tunnels-io/tunnels-mcp/internal/cli"
	"github.com/tunnels-io/tunnels-mcp/internal/mcp"
	"github.com/tunnels-io/tunnels-mcp/internal/tools"
	"github.com/tunnels-io/tunnels-mcp/internal/tunnels"
)

const serverName = "tunnels-mcp"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, serverName+": "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := tunnels.Load()
	switch {
	case errors.Is(err, tunnels.ErrTeamToken):
		return fmt.Errorf("that is a TEAM service token (tnlt_). A team token cannot act as a "+
			"personal account, and every tunnels.io route refuses one.\n"+
			"Create a personal token at tunnels.io/account/tokens and set TUNNELS_TOKEN to it.\n"+
			"Config file: %s", tunnels.ConfigPath())
	case errors.Is(err, tunnels.ErrNoToken):
		return fmt.Errorf("no tunnels.io token.\n"+
			"Set TUNNELS_TOKEN to a personal token (it starts tnl_).\n"+
			"Create one at tunnels.io/account/tokens under \"Connect an AI assistant\",\n"+
			"and tick the permissions you want this server to have:\n"+
			"  %s\n"+
			"Config file: %s", scopeList(), tunnels.ConfigPath())
	case err != nil:
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := tunnels.New(cfg.Base, cfg.Token)

	grants, err := tunnels.DiscoverSelf(ctx, client)
	if err != nil {
		var pe *tunnels.Error
		if errors.As(err, &pe) && pe.IsUnauthorized() {
			return errors.New("the tunnels.io token is invalid or revoked. " +
				"Create a new one at tunnels.io/account/tokens")
		}
		return fmt.Errorf("could not read the token's permissions: %w", err)
	}

	srv := mcp.New(serverName, tunnels.Version)
	srv.Instructions = "Read a tunnels.io account and put local ports online. " +
		"Only the tools this token was granted are listed."

	var offered, withheld []string
	for _, t := range tools.ReadTools {
		if grants.Has(t.Scope) {
			srv.Register(t.Registration(client))
			offered = append(offered, t.Name)
		} else {
			withheld = append(withheld, t.Name+" ("+t.Scope+")")
		}
	}

	runner := cli.NewRunner(cfg.CLI, cfg.Token)
	if cliErr := runner.Available(); cliErr != nil {
		fmt.Fprintln(os.Stderr, serverName+": "+cliErr.Error())
		fmt.Fprintln(os.Stderr, serverName+": start_tunnel and stop_tunnel are not available")
	} else {
		srv.Register(tools.StartTunnel(runner))
		srv.Register(tools.StopTunnel(runner))
		offered = append(offered, "start_tunnel", "stop_tunnel")
	}
	defer runner.StopAll()

	if !grants.Discovered {
		fmt.Fprintln(os.Stderr, serverName+": this tunnels.io deployment has no /api/v1/tokens/self route, "+
			"so permissions could not be read. Every tool is offered and the platform will refuse the ones you did not grant.")
	} else {
		fmt.Fprintf(os.Stderr, "%s: token %s (%s)\n", serverName, grants.Prefix, grants.Name)
	}
	fmt.Fprintf(os.Stderr, "%s: %d tools offered: %v\n", serverName, len(offered), offered)
	if len(withheld) > 0 {
		fmt.Fprintf(os.Stderr, "%s: %d tools withheld, the token lacks the scope: %v\n",
			serverName, len(withheld), withheld)
	}

	return srv.Serve(ctx, os.Stdin, os.Stdout)
}

func scopeList() string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tools.ReadTools {
		if !seen[t.Scope] {
			seen[t.Scope] = true
			out = append(out, t.Scope)
		}
	}
	return strings.Join(out, ", ")
}
