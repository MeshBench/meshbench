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
		uiTool("session_journal",
			"Every command this workbench has been driven with, newest last, including "+
				"the moment it was launched. Call this first when picking up a session "+
				"you did not start, or after anything that might have restarted the "+
				"process: a scenario does not survive a restart, and the journal is the "+
				"only thing that says so.",
			"session.journal", sObj(nil)),

		uiTool("session_ui_state",
			"Everything about the workbench window: which view is active, every panel "+
				"and whether it is docked or its own OS window, the transport state, "+
				"the map's centre and scale, the selection, and any status message. "+
				"Start here before driving the UI.",
			"ui.state", sObj(nil)),

		uiTool("session_set_view",
			"Switch the workbench to a view: Plan (build and site), Run (exercise and "+
				"watch), Debug (why did that happen), Validate (measured against a "+
				"real network). Each "+
				"opens its own panels.",
			"workspace.set", sObj(map[string]any{
				"name": sStr("Plan, Run, Debug or Validate"),
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

		uiTool("session_ui_keep_above",
			"Set whether windows opened from now on stay above the main one. Linux "+
				"only; on Wayland the ask makes them layer-shell windows that draw "+
				"their own title bar, so a machine may prefer it off. Omit the flag "+
				"to read the current one.",
			"ui.keep_above", sObj(map[string]any{"on": sBool("keep new windows above")})),
	}
}

// The mini companion: what a phone would do, without a phone.
func sessionCompanionTools() []Tool {
	return []Tool{
		uiTool("session_companion_connect",
			"Open a companion node's serial port and handshake with its firmware, the "+
				"way a phone app would. Claims the port exclusively, so anything else "+
				"attached is displaced. Returns the node's own name and radio settings.",
			"companion.connect", sObj(map[string]any{
				"node": sStr("the companion node's name"),
			}, "node")),

		uiTool("session_companion_disconnect",
			"Release a companion's serial port.",
			"companion.disconnect", sObj(map[string]any{"node": sStr("node name")}, "node")),

		uiTool("session_companion_state",
			"What the connected companion knows: its channels with their slot indices, "+
				"the messages it has received, and its contacts with hop counts.",
			"companion.state", sObj(map[string]any{"node": sStr("node name")}, "node")),

		uiTool("session_companion_send",
			"Send a text message from a companion to a channel, by channel name (for "+
				"example #sco). scope optionally sets the node's transport region first "+
				"- a region name, or <null> for unscoped - which is how a message is "+
				"put on a particular scope.",
			"companion.send", sObj(map[string]any{
				"node":    sStr("the sending companion"),
				"text":    sStr("the message"),
				"channel": sStr("channel name, e.g. #sco"),
				"scope":   sStr("region name, or <null> for unscoped"),
			}, "node", "text")),

		uiTool("session_companion_advert",
			"Send a flood advert from a companion, which is how other nodes learn it "+
				"exists and how it acquires contacts.",
			"companion.advert", sObj(map[string]any{"node": sStr("node name")}, "node")),

		uiTool("session_experiment_define",
			"Define a sweep: arms (named parameter sets), seeds, senders and timing. "+
				"An arm sets only what it varies - firmware version per role, path hash "+
				"mode (0,1,2 for 1,2,3 bytes per hop), loop detect, CAD - and leaves the "+
				"rest of the scenario alone.",
			"experiment.define", sObj(map[string]any{
				"arms":       map[string]any{"type": "array", "description": "objects: label, repeater_version, companion_version, path_hash_mode, loop_detect, cad"},
				"seeds":      map[string]any{"type": "array", "description": "run each arm once per seed; repeats of one seed are identical by design"},
				"senders":    map[string]any{"type": "array", "description": "companion node names that originate the burst"},
				"channel":    sStr("channel to send on, e.g. #sco"),
				"scope":      sStr("transport scope to send under"),
				"send_at_ms": map[string]any{"type": "integer", "description": "simulated instant every arm fires at"},
				"run_for_ms": map[string]any{"type": "integer", "description": "measurement window after the burst"},
			})),

		uiTool("session_experiment_start",
			"Run the sweep. Wipes persisted firmware state between every run, fires every "+
				"arm at the same simulated instant, and flags any run where nothing relayed.",
			"experiment.start", sObj(map[string]any{})),

		uiTool("session_experiment_state",
			"How far the sweep has got: phase, runs done of total, and the last few log lines.",
			"experiment.state", sObj(map[string]any{})),

		uiTool("session_experiment_results",
			"Per-run rows, per-arm averages, a warning when the seeds disagree by more than "+
				"the arms do, and - once finished - a verdict saying whether it really made a "+
				"difference, with an investigation of why not when it did not.",
			"experiment.results", sObj(map[string]any{})),

		uiTool("session_experiment_compare",
			"First divergence between two arms at one seed. Totals say something changed; "+
				"this says where, and reports when timing is identical.",
			"experiment.compare", sObj(map[string]any{
				"arm_a": sStr("first arm label"),
				"arm_b": sStr("second arm label"),
				"seed":  map[string]any{"type": "integer", "description": "which seed; the first if omitted"},
			}, "arm_a", "arm_b")),

		uiTool("session_experiment_export",
			"Write the sweep as a self-contained HTML report: verdict, matrix, one flood "+
				"shape per arm, every run, and the first divergence.",
			"experiment.export", sObj(map[string]any{})),

		uiTool("session_quit",
			"Close the workbench. The scenario lives in the process, so save a "+
				"project first if it matters - everything unsaved goes with it.",
			"app.quit", sObj(map[string]any{})),

		uiTool("session_firmware_installed",
			"List the firmware builds actually present on this machine: role, version, "+
				"board (native builds say so), size and path. This is what decides what "+
				"a node can run, and it is a different question from what has been "+
				"published - a build that failed to download halfway looks the same as "+
				"one in daily use from outside the cache.",
			"firmware.installed", sObj(map[string]any{})),

		uiTool("session_firmware_download",
			"Fetch a published build into the cache now, rather than waiting for a node "+
				"to need it. Wanted before working without a network, or when a run "+
				"should start on the instant. Returns immediately; poll "+
				"session_firmware_installed to see it arrive.",
			"firmware.download", sObj(map[string]any{
				"role":    sStr("role, e.g. simple_repeater or companion_radio"),
				"version": sStr("version as listed, e.g. repeater-v1.17.0"),
			})),

		uiTool("session_firmware_delete",
			"Delete one installed build. Identify it the way it is listed: role and "+
				"version, plus board for a board image. Deleting a build the scenario "+
				"is using leaves those nodes unable to start, and the failure arrives "+
				"when the run begins rather than here.",
			"firmware.delete", sObj(map[string]any{
				"version": sStr("version as listed, e.g. repeater-v1.17.0"),
				"role":    sStr("role as listed, e.g. simple_repeater"),
				"board":   sStr("board for a board image; omit for a native build"),
			})),

		uiTool("session_firmware_import",
			"Copy a local build into the cache so nodes can select it. Wanted for "+
				"anything never released, which is most of what is worth testing: a "+
				"branch build, a patched image, somebody else's binary. Omit board for "+
				"a native build for this machine.",
			"firmware.import", sObj(map[string]any{
				"path":    sStr("file to import"),
				"version": sStr("what to call it, e.g. v1.17.0-mybranch"),
				"role":    sStr("role, e.g. simple_repeater or companion_radio"),
				"board":   sStr("board this image is for; omit for a native build"),
			})),

		uiTool("session_firmware_wipe",
			"Delete every node's persistent firmware state: identity, preferences, "+
				"channels, contacts. Needed between the arms of a comparison, because "+
				"the firmware reads that state back at boot and the second run would "+
				"otherwise inherit the first one's.",
			"firmware.wipe", sObj(map[string]any{})),

		uiTool("session_companion_configure",
			"Apply the scenario's name, radio, transmit power and default scope to a "+
				"connected companion. Provisioning does not reach companions - it speaks "+
				"the repeater CLI, which a companion build does not have - so a companion "+
				"is unnamed, unscoped and on no particular frequency until this is called.",
			"companion.configure", sObj(map[string]any{"node": sStr("node name")}, "node")),
	}
}
