package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// RegisterSessionTools exposes a running workbench, if one is listening.
//
// The headless tools answer questions from their own engine; these drive the
// session the operator is looking at. Both are registered, and the difference
// is stated in every description — an assistant that cannot tell them apart
// will place a node in a simulation nobody can see.
//
// Registered unconditionally rather than only when a workbench is up: the
// socket comes and goes with the window, and a tool list that changed underneath
// a client mid-conversation is worse than a tool that says "no workbench is
// running".
func RegisterSessionTools(s *Server) error {
	for _, t := range sessionTools() {
		if err := s.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// sessionTools is every tool this server offers, assembled once so a test can
// walk it and check each one names a verb the session actually registers.
func sessionTools() []Tool {
	tools := []Tool{
		sessionDescribeTool(),
		sessionNodesTool(),
		sessionPlaceTool(),
		sessionMoveTool(),
		sessionDeleteTool(),
		sessionRunTool(),
		sessionFirmwareTool(),
		sessionConsoleTool(),
		sessionEventsTool(),
		sessionPresetTool(),
	}
	// The chrome verbs: view, panels, layouts, transport, navigation,
	// windows, tools, fleet, coverage, import. An agent that can change the
	// scenario but not the window can change what the operator is looking at
	// without ever seeing it.
	tools = append(tools, sessionUITools()...)
	tools = append(tools, sessionCompanionTools()...)
	return tools
}

// call reaches the workbench for one command.
//
// A fresh connection per call. The socket is local and the cost is a syscall;
// holding one open across a conversation would mean a stale handle every time
// the operator closes the window, which they do constantly.
func call(method string, params any) (string, error) {
	c, err := control.Dial()
	if err != nil {
		return "", fmt.Errorf("%w\n\nStart the workbench with `meshbench workbench`; "+
			"these tools drive a running session rather than their own simulation", err)
	}
	defer func() { _ = c.Close() }()

	raw, err := c.Call(method, params)
	if err != nil {
		return "", err
	}
	// Re-indented for a reader rather than a parser: an assistant reads these
	// as text, and a single line of dense JSON is where it stops noticing
	// fields.
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		return string(raw), nil
	}
	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		return string(raw), nil
	}
	return string(out), nil
}

// Schema helpers, named apart from the headless tools' own so the two sets can
// live in one package without either having to move.
func sObj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func sNum(desc string) map[string]any {
	return map[string]any{"type": "number", "description": desc}
}

func sStr(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func sBool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func sessionDescribeTool() Tool {
	return Tool{
		Name: "session_describe", Verb: "session.describe",
		Description: "What the running workbench currently holds: node count, simulated " +
			"clock, event count, how many nodes are on real firmware, and the study " +
			"region. Start here — it also confirms a workbench is running at all.",
		InputSchema: sObj(nil),
		Call: func(context.Context, json.RawMessage) (string, error) {
			return call("session.describe", nil)
		},
	}
}

func sessionNodesTool() Tool {
	return Tool{
		Name: "session_nodes", Verb: "nodes.list",
		Description: "Every node in the running session with its position, height, transmit " +
			"power, radio and — once a run has happened — what it has sent and heard.",
		InputSchema: sObj(nil),
		Call: func(context.Context, json.RawMessage) (string, error) {
			return call("nodes.list", nil)
		},
	}
}

func sessionPlaceTool() Tool {
	return Tool{
		Name: "session_place_node", Verb: "nodes.place",
		Description: "Place a node on the operator's map. kind is repeater, companion or " +
			"observer. This changes what they are looking at, so say what you placed " +
			"and why.",
		InputSchema: sObj(map[string]any{
			"kind":          sStr("repeater, companion or observer"),
			"name":          sStr("optional; one is generated otherwise"),
			"lat":           sNum("latitude"),
			"lon":           sNum("longitude"),
			"height_m":      sNum("antenna height above ground, default 10"),
			"tx_dbm":        sNum("transmit power; the board's maximum otherwise"),
			"firmware_role": sStr("MeshCore application, e.g. simple_repeater"),
		}, "lat", "lon"),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("nodes.place", json.RawMessage(args))
		},
	}
}

func sessionMoveTool() Tool {
	return Tool{
		Name: "session_move_node", Verb: "nodes.move",
		Description: "Move a node or change its mast height, and recompute. This is the " +
			"primary what-if: 400 m up the hill is frequently the whole answer.",
		InputSchema: sObj(map[string]any{
			"name":     sStr("the node to move"),
			"lat":      sNum("new latitude; omit to keep"),
			"lon":      sNum("new longitude; omit to keep"),
			"height_m": sNum("new antenna height; omit to keep"),
		}, "name"),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("nodes.move", json.RawMessage(args))
		},
	}
}

func sessionDeleteTool() Tool {
	return Tool{
		Name: "session_delete_node", Verb: "nodes.delete",
		Description: "Remove a node from the operator's session.",
		InputSchema: sObj(map[string]any{"name": sStr("the node to delete")}, "name"),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("nodes.delete", json.RawMessage(args))
		},
	}
}

func sessionRunTool() Tool {
	return Tool{
		Name: "session_run", Verb: "sim.run",
		Description: "Advance the simulation by a span of simulated time. The window is " +
			"blocked while this runs, so prefer seconds to hours.",
		InputSchema: sObj(map[string]any{
			"for_ms": sNum("simulated milliseconds to advance, default 10000"),
		}),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("sim.run", json.RawMessage(args))
		},
	}
}

func sessionFirmwareTool() Tool {
	return Tool{
		Name: "session_start_firmware", Verb: "firmware.start",
		Description: "Start real MeshCore on every node that runs firmware. Downloads the " +
			"build on first use. Until this is called, relay decisions are not the " +
			"firmware's own.",
		InputSchema: sObj(nil),
		Call: func(context.Context, json.RawMessage) (string, error) {
			return call("firmware.start", nil)
		},
	}
}

func sessionConsoleTool() Tool {
	return Tool{
		Name: "session_console", Verb: "console.type",
		Description: "Type a line at a node's real MeshCore CLI and return what it printed. " +
			"This is the firmware's own command interface — `advert`, `get flood.max`, " +
			"`set flood.max.advert 4` — and the reply is its own words.",
		InputSchema: sObj(map[string]any{
			"node":    sStr("which node"),
			"command": sStr("the CLI line, without a newline"),
		}, "node", "command"),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("console.type", json.RawMessage(args))
		},
	}
}

func sessionEventsTool() Tool {
	return Tool{
		Name: "session_events", Verb: "events.recent",
		Description: "Recent traffic from the running session: transmissions, receptions, " +
			"and why packets did not arrive. Each carries the cause in words.",
		InputSchema: sObj(map[string]any{
			"limit": sNum("how many of the most recent events, default 100"),
		}),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("events.recent", json.RawMessage(args))
		},
	}
}

func sessionPresetTool() Tool {
	return Tool{
		Name: "session_set_preset", Verb: "radio.preset",
		Description: "Apply a community radio preset (for example \"EU/UK (Narrow)\") to one " +
			"node or, with no node, to every transmitter. On nodes running firmware " +
			"this goes through their own CLI, so the firmware validates and persists it.",
		InputSchema: sObj(map[string]any{
			"preset": sStr("preset label, e.g. EU/UK (Narrow)"),
			"node":   sStr("one node, or omit for all transmitters"),
		}, "preset"),
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			return call("radio.preset", json.RawMessage(args))
		},
	}
}
