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
		// Objects arrive as objects.
		//
		// This used to unwrap a single-key one to a bare string, because that
		// is how the old socket's callers write a one-parameter verb. It also
		// made {"version": "x"} indistinguishable from {"node": "x"} to a verb
		// that takes both: firmware.set read the same string as the version,
		// the node and the role, matched no node, and said it had pinned
		// nothing. The unwrapping now happens where the verb knows which
		// parameter it wants - see soleString.
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
	// Counts first, because a caller polling in a loop wants the cheap
	// summary and not the whole world every second.
	out := map[string]any{
		"seq": s.Seq, "now_ms": s.NowMs, "playing": s.Playing, "seed": s.Seed,
		"nodes": len(s.Nodes), "links": len(s.Links), "areas": len(s.Areas),
		"events": s.EventTotal, "scores": len(s.Scores),
		"selected": selected, "status": s.Status,
	}

	// Then the state itself.
	//
	// This used to be the eleven scalars above and nothing else, which made
	// the socket a summary rather than a view of the session: endpoints,
	// jobs, residuals, the firmware library, the console, node statistics and
	// everything else existed in the snapshot the panels draw from and were
	// invisible to anything driving the workbench. A caller could not see
	// what it had just asked for, and neither could a test.
	out["real_firmware"] = s.RealFirmware
	out["firmware_running"] = s.FirmwareRunning
	out["firmware_starting"] = s.FirmwareStarting
	out["pending_play"] = s.PendingPlay
	out["step_ms"] = s.StepMs
	out["margin_km"] = s.MarginKm

	if len(s.Endpoints) > 0 {
		eps := make([]map[string]any, 0, len(s.Endpoints))
		for _, e := range s.Endpoints {
			eps = append(eps, map[string]any{
				"node": e.Node, "addr": e.Addr, "kind": e.Kind,
				"attached": e.Attached,
			})
		}
		out["endpoints"] = eps
	}
	if len(s.Jobs) > 0 {
		jobs := make([]map[string]any, 0, len(s.Jobs))
		for _, j := range s.Jobs {
			jobs = append(jobs, map[string]any{
				"id": j.ID, "what": j.What, "done": j.Done,
				"total": j.Total, "finished": j.Finished,
			})
		}
		out["jobs"] = jobs
	}
	if s.Residuals != nil {
		out["residuals"] = map[string]any{
			"matched": s.Residuals.Matched, "unmatched": s.Residuals.Unmatched,
			"median_db": s.Residuals.MedianDB, "iqr_db": s.Residuals.IQRdB,
		}
	}
	if len(s.Builds) > 0 {
		out["builds"] = len(s.Builds)
	}
	if len(s.Stats) > 0 {
		out["node_stats"] = len(s.Stats)
	}
	if len(s.Console) > 0 {
		out["console_node"] = s.ConsoleNode
		out["console_lines"] = len(s.Console)
	}
	if len(s.Provisioning) > 0 {
		out["provisioning_node"] = s.ProvisioningNode
		out["provisioning_lines"] = len(s.Provisioning)
	}
	if len(s.Experiment) > 0 {
		out["experiment_arms"] = len(s.Experiment)
		out["experiment_warning"] = s.ExperimentWarning
	}
	if len(s.Sends) > 0 {
		out["scheduled_sends"] = len(s.Sends)
	}
	if len(s.Assertions) > 0 {
		out["assertions"] = len(s.Assertions)
	}
	if s.Import != nil {
		out["import"] = map[string]any{
			"url": s.Import.URL, "records": s.Import.Records,
			"nodes": s.Import.Nodes,
		}
	}
	if s.Coverage != nil {
		out["coverage"] = true
	}
	if s.Waterfall != nil {
		out["waterfall"] = true
	}
	return out
}
