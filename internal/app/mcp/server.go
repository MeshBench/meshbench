// Package mcp exposes the engine to an AI client over the Model Context
// Protocol.
//
// Over stdio, as a process the client spawns. That is not an implementation
// detail: MeshcoreSim runs standalone and this must not become a service. There
// is no port, no session, nothing to deploy and nothing listening when the
// client is not running.
//
// The tools here deliberately return the *reasoning*, not just the number. An
// assistant handed "margin: 4.2 dB" will state it as fact; one handed the
// margin, the limiting direction, the path loss and the model's own known
// optimism can say something an engineer would accept.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Protocol version this server speaks.
const protocolVersion = "2024-11-05"

// Tool is one callable.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`

	// Verb is the verb Call reaches, named so it can be checked against what
	// the session registers. It was not, and this server shipped a
	// session_journal tool calling session.journal - a verb no version of this
	// tree registers - because the name lived only inside the closure below
	// where nothing could compare it to anything.
	Verb string `json:"-"`

	// Call runs it. Returning an error produces a tool error the model can
	// read and react to, rather than a transport failure that ends the session
	// — a wrong argument should be correctable, not fatal.
	Call func(ctx context.Context, args json.RawMessage) (string, error) `json:"-"`
}

// Server speaks MCP over a reader and writer.
type Server struct {
	Name    string
	Version string

	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

func NewServer(name, version string) *Server {
	return &Server{Name: name, Version: version, tools: map[string]Tool{}}
}

// Register adds a tool. Duplicate names are an error rather than a silent
// replacement.
func (s *Server) Register(t Tool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tools[t.Name]; exists {
		return fmt.Errorf("mcp: tool %q is already registered", t.Name)
	}
	s.tools[t.Name] = t
	s.order = append(s.order, t.Name)
	return nil
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve runs until the input ends or the context is cancelled.
//
// Line-delimited JSON, which is what MCP stdio transport uses. The scanner
// buffer is raised because a tool result carrying a coverage summary is easily
// past the 64 kB default, and the failure mode there is a truncated line that
// decodes as malformed rather than as too long.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := enc.Encode(response{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "parse error: " + err.Error()},
			}); err != nil {
				return err
			}
			continue
		}

		// A notification has no id and takes no reply. Answering one is a
		// protocol violation that some clients tolerate and others do not.
		if len(req.ID) == 0 {
			continue
		}

		result, rpcErr := s.dispatch(ctx, req)
		if err := enc.Encode(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}, nil

	case "tools/list":
		s.mu.RLock()
		defer s.mu.RUnlock()
		list := make([]Tool, 0, len(s.order))
		for _, n := range s.order {
			list = append(list, s.tools[n])
		}
		return map[string]any{"tools": list}, nil

	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "bad params: " + err.Error()}
		}
		s.mu.RLock()
		t, ok := s.tools[p.Name]
		s.mu.RUnlock()
		if !ok {
			return nil, &rpcError{Code: -32601, Message: fmt.Sprintf("no tool named %q", p.Name)}
		}

		text, err := t.Call(ctx, p.Arguments)
		if err != nil {
			// A tool error is content, not transport. The model can read it and
			// try again; a JSON-RPC error would look like the server broke.
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}, nil
		}
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		}, nil

	case "ping":
		return map[string]any{}, nil

	default:
		return nil, &rpcError{Code: -32601, Message: "unknown method " + req.Method}
	}
}

// ErrMissingArgument is returned by tools when a required argument is absent.
var ErrMissingArgument = errors.New("missing required argument")
