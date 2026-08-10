package ui

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// commandInvalidatesRun reports whether a CLI line changes the network out from
// under the results already collected.
func commandInvalidatesRun(cmd string) bool {
	// Region changes belong here too: scoping alters which packets a node will
	// relay at all, so results either side of one are not comparable.
	for _, p := range []string{"set freq", "set bw", "set sf", "set cr", "set radio", "set tx",
		"region put", "region allowf", "region denyf", "region default", "region remove"} {
		if strings.HasPrefix(strings.TrimSpace(cmd), p) {
			return true
		}
	}
	return false
}

// fleetState is the Fleet commands window: one CLI line, many real consoles.
type fleetState struct {
	target  string // all | repeaters | companions | selection | filter
	command string
	// results is the last dispatch, one row per node, replies verbatim.
	results []fleetResult
	history []string
	// confirmed arms the second press for a run-invalidating command.
	confirmed bool
}

type fleetResult struct {
	node  string
	reply string
	err   string
}

// fleetQuick are the operations an operator reaches for often enough to
// deserve a button. Each is a real CLI line, shown before it is sent — the
// button is a shortcut for typing, not a private API the firmware never sees.
// Grouped the way an operator thinks — flood policy, regions, radio, and
// asking questions — because sixteen buttons in a flat grid were sixteen
// things to re-read every time.
var fleetQuick = []struct {
	group string
	rows  []struct{ label, cmd string }
}{
	{"Flood", []struct{ label, cmd string }{
		{"get flood settings", "get flood.max"},
		{"deny unscoped flooding", "set flood.max.unscoped 0"},
		{"allow unscoped flooding", "set flood.max.unscoped 64"},
	}},
	// Regions. Named rather than inferred: which region a fleet belongs to is
	// an operator's decision about their network, and a tool that guessed would
	// scope traffic somewhere nobody chose.
	{"Regions", []struct{ label, cmd string }{
		{"list regions", "region"},
		{"read default scope", "region default"},
		{"put region", "region put "},
		{"allow flooding for region", "region allowf "},
		{"deny flooding for region", "region denyf "},
		{"set default scope", "region default "},
		{"clear default scope", "region default <null>"},
		{"save regions", "region save"},
	}},
	{"Radio", []struct{ label, cmd string }{
		{"read frequency", "get freq"},
		{"read spreading factor", "get sf"},
		{"read bandwidth", "get bw"},
		{"read tx power", "get tx"},
	}},
	{"Info", []struct{ label, cmd string }{
		{"advert now", "advert"},
		{"version", "ver"},
		{"read-only on", "set allow.read.only on"},
		{"read-only off", "set allow.read.only off"},
	}},
}

// drawFleetWindow sends one command to many nodes and shows every reply.
//
// This is how a mesh is administered rather than a node: set a region, cap
// flooding, or push a preset across forty repeaters, with each node's own
// firmware parsing the command and each reply attributed. A node that rejects
// the command says so here in its own words, which is the entire reason to run
// real firmware.
func (a *App) drawFleetWindow() {
	if !a.winFleet {
		return
	}
	imgui.SetNextWindowSizeV(imgui.NewVec2(640, 420), imgui.CondFirstUseEver)
	open := a.winFleet
	if imgui.BeginV("Fleet commands", &open, 0) {
		a.drawFleetBody()
	}
	imgui.End()
	a.winFleet = open
}

func (a *App) drawFleetBody() {
	if a.eng == nil || a.eng.FirmwareCount() == 0 {
		imgui.TextWrapped("Fleet commands talk to each node's real CLI, so they need real " +
			"firmware running. Start it from the Simulation menu.")
		return
	}

	if a.fleet.target == "" {
		a.fleet.target = "repeaters"
	}
	imgui.SetNextItemWidth(140)
	if imgui.BeginCombo("target", a.fleet.target) {
		for _, t := range []string{"all", "repeaters", "companions", "selection", "filter"} {
			if imgui.SelectableBool(t) {
				a.fleet.target = t
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	targets := a.fleetTargets()
	imgui.TextDisabled(fmt.Sprintf("%d nodes", len(targets)))

	imgui.SetNextItemWidth(-70)
	entered := imgui.InputTextWithHint("##fleetcmd", "a MeshCore CLI line, sent to every target",
		&a.fleet.command, imgui.InputTextFlagsEnterReturnsTrue, nil)
	imgui.SameLine()
	// Some commands silently invalidate a run. Changing radio parameters
	// mid-flight means every result before and after came from different
	// networks, and a comparison across that boundary is meaningless.
	invalidates := commandInvalidatesRun(a.fleet.command)
	needsArg := strings.HasSuffix(a.fleet.command, " ")
	if needsArg {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		imgui.TextWrapped("This command needs a region name after it. Nothing is guessed: " +
			"which region a fleet belongs to is a decision about your network.")
		imgui.PopStyleColor()
	}
	if (imgui.Button("send") || entered) && a.fleet.command != "" && !needsArg {
		if invalidates && !a.fleet.confirmed {
			a.fleet.confirmed = true
		} else {
			a.fleet.confirmed = false
			a.fleetSend(targets, a.fleet.command)
		}
	}
	if invalidates {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		if a.fleet.confirmed {
			imgui.TextWrapped(fmt.Sprintf("This changes the radio on %d nodes and invalidates "+
				"this run. Press send again to confirm.", len(targets)))
		} else {
			imgui.TextWrapped(fmt.Sprintf("Changes the radio on %d nodes — invalidates this run.",
				len(targets)))
		}
		imgui.PopStyleColor()
	}

	for _, g := range fleetQuick {
		if !imgui.CollapsingHeaderBoolPtr(g.group, nil) {
			continue
		}
		for i, q := range g.rows {
			if i%3 != 0 {
				imgui.SameLine()
			}
			if imgui.SmallButton(q.label) {
				// Into the box rather than straight out the door: the operator
				// sees the exact line, can edit it, and presses send. A button
				// that silently issues commands to forty nodes is a footgun
				// with a label.
				a.fleet.command = q.cmd
			}
			if imgui.IsItemHovered() {
				imgui.SetTooltip(q.cmd)
			}
		}
	}

	if len(a.fleet.history) > 0 {
		if imgui.BeginCombo("##hist", "history") {
			for i := len(a.fleet.history) - 1; i >= 0; i-- {
				if imgui.SelectableBool(a.fleet.history[i]) {
					a.fleet.command = a.fleet.history[i]
				}
			}
			imgui.EndCombo()
		}
	}

	imgui.Separator()
	if len(a.fleet.results) == 0 {
		imgui.TextDisabled("replies appear here, one row per node, in the firmware's own words")
		return
	}
	if !imgui.BeginTableV("##fleetresults", 2,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumnV("node", imgui.TableColumnFlagsWidthFixed, 130, 0)
	imgui.TableSetupColumnV("reply", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableHeadersRow()
	for _, r := range a.fleet.results {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(r.node)
		imgui.TableSetColumnIndex(1)
		switch {
		case r.err != "":
			imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
			imgui.TextWrapped(r.err)
			imgui.PopStyleColor()
		case r.reply == "":
			imgui.TextDisabled("(no reply)")
		default:
			imgui.TextWrapped(r.reply)
		}
	}
	imgui.EndTable()
}

// fleetTargets resolves the target picker to node names, firmware-running only.
func (a *App) fleetTargets() []string {
	var out []string
	for i := range a.Nodes {
		n := &a.Nodes[i]
		en, ok := a.eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		switch a.fleet.target {
		case "repeaters":
			if n.Kind != scenario.SimpleRepeater && n.Kind != scenario.AdvancedRepeater {
				continue
			}
		case "companions":
			if n.Kind != scenario.Companion {
				continue
			}
		case "selection":
			if i != a.selected && i != a.linkTo {
				continue
			}
		case "filter":
			want := strings.ToLower(strings.TrimSpace(a.nodeFilter))
			if want != "" && !strings.Contains(strings.ToLower(n.Name), want) {
				continue
			}
		}
		out = append(out, n.Name)
	}
	return out
}

// fleetSend types one line at every target's console and gathers the replies.
func (a *App) fleetSend(targets []string, cmd string) {
	a.fleet.results = a.fleet.results[:0]
	if len(a.fleet.history) == 0 || a.fleet.history[len(a.fleet.history)-1] != cmd {
		a.fleet.history = append(a.fleet.history, cmd)
	}

	// Marks first, so each node's reply is only what arrived after its command.
	marks := map[string]int{}
	for _, name := range targets {
		buf := a.consoleBufFor(name)
		if buf == nil {
			a.fleet.results = append(a.fleet.results, fleetResult{node: name, err: "no console"})
			continue
		}
		marks[name] = buf.mark()
		if err := a.typeAt(name, cmd); err != nil {
			a.fleet.results = append(a.fleet.results, fleetResult{node: name, err: err.Error()})
			delete(marks, name)
		}
	}

	// One second of simulated time for every reply to be written. The engine is
	// deterministic, so this is not a race being papered over — the node reads
	// its serial input on its next loop and replies within a tick or two.
	a.stepEngine(100)

	for _, name := range targets {
		m, ok := marks[name]
		if !ok {
			continue
		}
		reply := strings.TrimSpace(strings.Join(a.consoleBufFor(name).linesSince(m), "\n"))
		a.fleet.results = append(a.fleet.results, fleetResult{node: name, reply: reply})
	}
	a.status = fmt.Sprintf("%q sent to %d nodes", cmd, len(marks))
}

// consoleBufFor returns the node's console buffer, attaching one if the node
// runs firmware and has never been looked at.
func (a *App) consoleBufFor(name string) *consoleBuf {
	en, ok := a.eng.NodeByName(name)
	if !ok || en.Firmware == nil {
		return nil
	}
	if a.consoles == nil {
		a.consoles = map[string]*consoleBuf{}
	}
	buf, seen := a.consoles[name]
	if !seen || buf.bridge != en.Firmware.Bridge {
		buf = &consoleBuf{bridge: en.Firmware.Bridge}
		a.consoles[name] = buf
		en.Firmware.Bridge.Console(buf)
	}
	return buf
}

// typeAt sends one CRLF-terminated line to a node's serial input.
func (a *App) typeAt(name, line string) error {
	en, ok := a.eng.NodeByName(name)
	if !ok || en.Firmware == nil {
		return fmt.Errorf("%s runs no firmware", name)
	}
	return en.Firmware.Bridge.Type([]byte(line + "\r\n"))
}
