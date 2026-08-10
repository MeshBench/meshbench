package mcp

import (
	"context"
	"encoding/json"
)

// The UI half of the session tools: the chrome, not the scenario.
//
// An agent that can place a node but cannot switch view, open a panel or
// press play is an agent that can change the workbench and never see it.
// These mirror the control socket's UI verbs one for one.

func uiTool(name, desc, method string, schema map[string]any) Tool {
	return Tool{
		Name: name, Description: desc, InputSchema: schema,
		Call: func(_ context.Context, args json.RawMessage) (string, error) {
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			return call(method, args)
		},
	}
}

func sessionUITools() []Tool {
	return []Tool{
		uiTool("session_ui_state",
			"Everything about the workbench window: which view is active, every panel "+
				"and whether it is docked or its own OS window, the transport state, "+
				"the map's centre and scale, the selection, and any status message. "+
				"Start here before driving the UI.",
			"ui.state", sObj(nil)),

		uiTool("session_set_view",
			"Switch the workbench to a view: Plan (build and site), Run (exercise and "+
				"watch), Debug (why did that happen), Verify (is it still true). Each "+
				"opens its own panels.",
			"workspace.set", sObj(map[string]any{
				"name": sStr("Plan, Run, Debug or Verify"),
			}, "name")),

		uiTool("session_panels",
			"List every panel: whether it is open, docked, or popped out to its own OS "+
				"window.",
			"panels.list", sObj(nil)),

		uiTool("session_panel",
			"Open or close a panel by name (Inspector, Nodes, Link, Budget, Waterfall, "+
				"Packet timeline, Events, Scoreboard, Console, Schedule, Compare, "+
				"Validate, Live feed, Import, Boundary, Planning, Fleet, Energy).",
			"panel.open", sObj(map[string]any{
				"name": sStr("panel name"),
				"open": map[string]any{"type": "boolean", "description": "false to close"},
			}, "name")),

		uiTool("session_pop_out_panel",
			"Make a panel its own OS window, which the operator can move to another "+
				"monitor. Requires the X11 mode; on native Wayland the platform forbids it.",
			"panel.pop_out", sObj(map[string]any{"name": sStr("panel name")}, "name")),

		uiTool("session_dock_panel",
			"Put a popped-out panel back into the main window, where it came from.",
			"panel.dock", sObj(map[string]any{"name": sStr("panel name")}, "name")),

		uiTool("session_save_layout",
			"Save the current arrangement — every panel's place, popped-out ones "+
				"included — under a name the operator can return to.",
			"view.save", sObj(map[string]any{"name": sStr("layout name")}, "name")),

		uiTool("session_load_layout",
			"Restore a saved layout by name.",
			"view.load", sObj(map[string]any{"name": sStr("layout name")}, "name")),

		uiTool("session_layouts",
			"List saved layouts.",
			"view.list", sObj(nil)),

		uiTool("session_delete_layout",
			"Delete a saved layout.",
			"view.delete", sObj(map[string]any{"name": sStr("layout name")}, "name")),

		uiTool("session_play",
			"Start the simulation running (the play button).",
			"sim.play", sObj(nil)),

		uiTool("session_pause",
			"Pause the simulation.",
			"sim.pause", sObj(nil)),

		uiTool("session_step",
			"Advance the simulation by a number of ticks (default 20, about 200 ms of "+
				"simulated time) without leaving it running.",
			"sim.step", sObj(map[string]any{
				"ticks": sNum("ticks of 10 ms each"),
			})),

		uiTool("session_speed",
			"Set how fast simulated time runs against wall time (0.25, 1, 4, 12). "+
				"Pinned to 1 while a companion client is attached.",
			"sim.speed", sObj(map[string]any{"factor": sNum("speed multiplier")}, "factor")),

		uiTool("session_map",
			"Point the operator's map at a place: centre latitude and longitude, and "+
				"optionally the scale in metres per pixel.",
			"map.centre", sObj(map[string]any{
				"lat":              sNum("latitude, north positive"),
				"lon":              sNum("longitude, east positive"),
				"metres_per_pixel": sNum("scale; smaller is closer in"),
			}, "lat", "lon")),

		uiTool("session_map_fit",
			"Fit the map to every node in the scenario.",
			"map.fit", sObj(nil)),

		uiTool("session_filter_nodes",
			"Type into the map's node filter: matching nodes are highlighted and the "+
				"node list narrows. Empty text clears it.",
			"map.filter", sObj(map[string]any{"text": sStr("filter text")}, "text")),

		uiTool("session_select_node",
			"Select a node, as clicking it does. With add_to_link true it becomes the "+
				"far end of a link, which is what fills the Link and Budget panels.",
			"nodes.select", sObj(map[string]any{
				"name":        sStr("node name"),
				"add_to_link": map[string]any{"type": "boolean", "description": "make it the second end"},
			}, "name")),

		uiTool("session_set_tool",
			"Choose what a click on the map does: select, move, repeater, companion, "+
				"SDR observer, custom emitter.",
			"tool.set", sObj(map[string]any{"tool": sStr("tool name")}, "tool")),

		uiTool("session_open_window",
			"Open a single-instance window: Preferences, Provisioning, Firmware "+
				"library, Nodes & settings.",
			"window.open", sObj(map[string]any{"name": sStr("window name")}, "name")),

		uiTool("session_node_window",
			"Open a node's own window - console, settings, stats and activity for that "+
				"one node.",
			"node.window", sObj(map[string]any{"name": sStr("node name")}, "name")),

		uiTool("session_fleet_command",
			"Send a MeshCore CLI line to every firmware-running repeater, or to one "+
				"node. The replies land in the Fleet panel, each attributed.",
			"fleet.send", sObj(map[string]any{
				"command": sStr("a MeshCore CLI line, e.g. advert"),
				"node":    sStr("one node, or omit for the whole fleet"),
			}, "command")),

		uiTool("session_coverage",
			"Compute a coverage overlay: best (best server), gaps, redundancy, or node "+
				"(from the selected node).",
			"coverage.start", sObj(map[string]any{
				"mode": sStr("best, gaps, redundancy or node"),
			}, "mode")),

		uiTool("session_clear_coverage",
			"Remove the coverage overlay from the map.",
			"coverage.clear", sObj(nil)),

		uiTool("session_import_source",
			"Set the Import panel's source and URL (corescope, beacon, saved, file).",
			"import.set_source", sObj(map[string]any{
				"source": sStr("corescope, beacon, saved or file"),
				"url":    sStr("base URL or path"),
				"token":  sStr("token, if the deployment needs one"),
			})),

		uiTool("session_import_fetch",
			"Fetch a preview from the configured import source. Nothing enters the "+
				"scenario until it is committed.",
			"import.fetch", sObj(nil)),

		uiTool("session_import_commit",
			"Commit the fetched preview into the scenario with a merge strategy: "+
				"add-only-new, replace-matching or replace-all.",
			"import.commit", sObj(map[string]any{
				"strategy": sStr("add-only-new, replace-matching or replace-all"),
			})),

		uiTool("session_ui_scale",
			"Set the UI scale (1.0 is default, 1.5 is half again as large). Omit the "+
				"factor to read the current one.",
			"ui.scale", sObj(map[string]any{"factor": sNum("scale multiplier")})),
	}
}
