package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type Handler func(ctx context.Context, args json.RawMessage) *CallToolResult

type Registration struct {
	Tool    Tool
	Handler Handler
}

type Server struct {
	Name         string
	Version      string
	Instructions string

	mu    sync.RWMutex
	tools []Registration
	index map[string]Handler
}

func New(name, version string) *Server {
	return &Server{Name: name, Version: version, index: map[string]Handler{}}
}

func (s *Server) Register(r Registration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, r)
	s.index[r.Tool.Name] = r.Handler
}

func (s *Server) ToolNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.tools))
	for _, r := range s.tools {
		out = append(out, r.Tool.Name)
	}
	return out
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	enc := json.NewEncoder(out)
	var writeMu sync.Mutex
	write := func(resp Response) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = enc.Encode(resp)
	}

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			write(Response{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &RPCError{Code: CodeParse, Message: "invalid JSON"}})
			continue
		}
		s.dispatch(ctx, req, write)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	return nil
}

func (s *Server) dispatch(ctx context.Context, req Request, write func(Response)) {
	notify := req.IsNotification()
	reply := func(result any, rpcErr *RPCError) {
		if notify {
			return
		}
		write(Response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr})
	}

	switch req.Method {
	case "initialize":
		reply(InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities:    Capabilities{Tools: &ToolsCapability{ListChanged: false}},
			ServerInfo:      ServerInfo{Name: s.Name, Version: s.Version},
			Instructions:    s.Instructions,
		}, nil)

	case "notifications/initialized", "notifications/cancelled":

	case "ping":
		reply(map[string]any{}, nil)

	case "tools/list":
		s.mu.RLock()
		list := make([]Tool, 0, len(s.tools))
		for _, r := range s.tools {
			list = append(list, r.Tool)
		}
		s.mu.RUnlock()
		reply(ListToolsResult{Tools: list}, nil)

	case "tools/call":
		var p CallToolParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(nil, &RPCError{Code: CodeInvalidParams, Message: "invalid params"})
			return
		}
		s.mu.RLock()
		h, ok := s.index[p.Name]
		s.mu.RUnlock()
		if !ok {
			reply(nil, &RPCError{Code: CodeMethodNotFound, Message: "unknown tool: " + p.Name})
			return
		}
		res := func() (r *CallToolResult) {
			defer func() {
				if p := recover(); p != nil {
					r = Errorf(fmt.Sprintf("the tool failed unexpectedly: %v", p))
				}
			}()
			return h(ctx, p.Arguments)
		}()
		reply(res, nil)

	default:
		reply(nil, &RPCError{Code: CodeMethodNotFound, Message: "unknown method: " + req.Method})
	}
}
