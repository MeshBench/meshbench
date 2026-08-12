// The control socket, over the store's verbs.
//
// The imgui workbench queues control commands and performs them on the frame
// thread, because its state is the frame thread. This one does not need to:
// every verb already runs on the goroutine that owns the world, and Do is how
// a click reaches it too. So a script and a gesture take the same path, and
// there is no second route to keep in step.
//
// That also makes the interface testable from outside without a mouse, which
// is the gap that has cost this project the most: rendering can be checked
// from a screenshot, behaviour cannot.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/A13xB0/meshcoresim/internal/control"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// ServeControl opens the socket and answers from the store.
func ServeControl(ctx context.Context, st *state.Store) (*control.Server, error) {
	srv, err := control.Listen(func(method string, raw json.RawMessage) (any, error) {
		// Two methods the socket owns, because they are about the socket's
		// view of the application rather than about the world.
		switch method {
		case "session.verbs":
			v := append([]string(nil), st.Verbs()...)
			sort.Strings(v)
			return map[string]any{"verbs": v}, nil
		case "session.snapshot":
			return snapshotSummary(st.Snapshot()), nil
		}
		params, err := decodeParams(raw)
		if err != nil {
			return nil, err
		}
		return st.Do(ctx, method, params)
	})
	if err != nil {
		return nil, err
	}
	// Pumped from a worker rather than from the frame loop. The imgui
	// workbench has to pump on the frame thread because its state is the frame
	// thread; here every verb lands on the store's goroutine whichever way it
	// was called, so a slow verb delays the socket rather than the window.
	go func() {
		t := time.NewTicker(5 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				srv.Pump()
			}
		}
	}()
	fmt.Fprintln(os.Stderr, "control socket:", srv.Path())
	return srv, nil
}

// decodeParams turns the wire's JSON into what a verb expects.
//
// Verbs take Go values - a string, a []string, a map - because they are also
// called from the interface, where there is no JSON. So the socket unwraps the
// common shapes rather than making every verb learn about json.RawMessage.
func decodeParams(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case map[string]any:
		// A single-key object naming the parameter is how the old socket's
		// callers write it: {"name": "..."} rather than a bare string.
		if len(t) == 1 {
			for _, only := range t {
				if s, ok := only.(string); ok {
					return s, nil
				}
			}
		}
		return t, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return t, nil
			}
			out = append(out, s)
		}
		return out, nil
	}
	return v, nil
}

// snapshotSummary is what a caller usually wants to assert on: the numbers,
// not the geometry.
func snapshotSummary(s *state.Snapshot) map[string]any {
	if s == nil {
		return map[string]any{"nodes": 0}
	}
	selected := ""
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			selected = s.Nodes[i].Name
			break
		}
	}
	return map[string]any{
		"seq": s.Seq, "now_ms": s.NowMs, "playing": s.Playing, "seed": s.Seed,
		"nodes": len(s.Nodes), "links": len(s.Links), "areas": len(s.Areas),
		"events": s.EventTotal, "scores": len(s.Scores),
		"selected": selected, "status": s.Status,
	}
}
