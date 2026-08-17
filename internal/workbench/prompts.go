// The questions the workbench asks before it can act: which two nodes to
// plan between, and which build each role should run.
//
// They are here rather than in the panels that trigger them because a prompt
// belongs to the window it appears over, not to the button that asked.
package workbench

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// selectedNames is who is selected right now.
func selectedNames(s *state.Snapshot) []string {
	if s == nil {
		return nil
	}
	var out []string
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			out = append(out, s.Nodes[i].Name)
		}
	}
	return out
}

// askForRouteEnds gets the two ends the route search needs, then selects them
// and runs it - so the selection ends up saying what was planned, which is
// what somebody who selected the nodes by hand would have seen.
func askForRouteEnds(ctx context.Context, st *state.Store, ask *shell.Prompt,
	sel []string) {
	s := st.Snapshot()
	if s == nil || len(s.Nodes) < 2 {
		return
	}
	names := make([]string, 0, len(s.Nodes))
	for i := range s.Nodes {
		names = append(names, s.Nodes[i].Name)
	}
	run := func(from, to string) {
		go func() {
			_, _ = st.Do(ctx, "nodes.select_many", []string{from, to})
			_, _ = st.Do(ctx, "plan.routes", nil)
		}()
	}
	if len(sel) == 1 {
		from := sel[0]
		ask.Choose("Plan a route from "+from+" to", "filter",
			except(names, from), func(to string) { run(from, to) })
		return
	}
	ask.Choose("Plan a route from", "filter", names, func(from string) {
		ask.Choose("Plan a route from "+from+" to", "filter",
			except(names, from), func(to string) { run(from, to) })
	})
}

func except(names []string, drop string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}

// rolesNeeding reads firmware.needed's answer.
func rolesNeeding(res any) []roleNeed {
	m, _ := res.(map[string]any)
	raw, _ := m["roles"].([]any)
	var out []roleNeed
	for _, r := range raw {
		rm, _ := r.(map[string]any)
		n := roleNeed{role: fmt.Sprint(rm["role"])}
		switch v := rm["nodes"].(type) {
		case int:
			n.nodes = v
		case float64:
			n.nodes = int(v)
		}
		if cs, ok := rm["choices"].([]string); ok {
			n.choices = cs
		}
		out = append(out, n)
	}
	return out
}

// askForFirmware asks what each role should run, then starts the run.
//
// One question per role, in turn, because two modal questions at once is a
// state nobody can reason about. A role with nothing installed is said aloud
// rather than skipped silently - "the run did not start" and "there is no
// companion build on this machine" are not the same problem.
func askForFirmware(ctx context.Context, st *state.Store, ask *shell.Prompt,
	roles []roleNeed) {
	if len(roles) == 0 {
		go func() { _, _ = st.Do(ctx, "sim.start", nil) }()
		return
	}
	r := roles[0]
	rest := roles[1:]
	if len(r.choices) == 0 {
		go func() {
			_, _ = st.Do(ctx, "ui.said", fmt.Sprintf(
				"%d %s have nothing to run, and no build for them is installed - "+
					"download one in the Firmware library",
				r.nodes, readableRole(r.role)))
		}()
		return
	}
	title := fmt.Sprintf("What should the %d %s run?", r.nodes, readableRole(r.role))
	ask.Post(func(a *shell.Prompt) {
		a.Choose(title, "filter", r.choices, func(version string) {
			go func() {
				_, _ = st.Do(ctx, "firmware.set",
					map[string]any{"role": r.role, "version": version})
				askForFirmware(ctx, st, a, rest)
			}()
		})
	})
}

// readableRole says a role the way somebody would.
//
// The token is what firmware.set matches on and is not for reading:
// "simple_room_server" in a question about what to run is the program
// talking to itself.
func readableRole(role string) string {
	switch role {
	case "simple_repeater":
		return "repeaters"
	case "advanced_repeater":
		return "advanced repeaters"
	case "companion_radio":
		return "companions"
	case "simple_room_server":
		return "room servers"
	}
	return strings.ReplaceAll(role, "_", " ") + " nodes"
}
