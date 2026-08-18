// Reading a node back, and reconciling what the rules want against it.
//
// Provisioning used to be one script, typed and forgotten. This is read,
// decide, write, verify: ask every target what it actually holds, work out
// what the active rules want, send only what differs, and record every
// reply. A run that changes nothing sends nothing - which is also how the
// clock's forward-only rule stops being a hazard: a node whose time is
// already set is not sent `time` again.
package session

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/console"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provision"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// provisioningContext is everything a provisioning run needs from the store,
// gathered in one round trip so the rest of the run can work entirely off
// data it already holds - Sim's maps (consoles, and anything World holds) are
// touched only from verbs, never from the goroutine that sends the bytes, so
// two things can never race over them.
type provisioningContext struct {
	Nodes    []scenario.Node
	Bufs     map[string]*console.Buf
	Areas    map[string][]string
	Selected map[string]bool
	Rules    []provision.Rule
	// KindOf is every target's kind, for the one loop that only has a name
	// and needs to know whether it is looking at a companion before it does
	// anything CLI-shaped with it.
	KindOf map[string]scenario.Kind
}

func registerProvisioningContext(st *state.Store, s *Sim) {
	st.Handle("provisioning.context", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		only := stringSetField(p, "nodes")
		selected := map[string]bool{}
		for _, n := range w.Nodes {
			selected[n.Name] = n.Selected
		}

		out := provisioningContext{
			Bufs: map[string]*console.Buf{}, Areas: map[string][]string{},
			Selected: selected, Rules: s.activeRules(), KindOf: map[string]scenario.Kind{},
		}
		for _, n := range s.nodes {
			if only != nil && !only[n.Name] {
				continue
			}
			en, ok := s.eng.NodeByName(n.Name)
			if !ok || en.Firmware == nil {
				continue
			}
			buf, err := s.consoleFor(n.Name)
			if err != nil {
				continue
			}
			out.Nodes = append(out.Nodes, n)
			out.Bufs[n.Name] = buf
			out.Areas[n.Name] = areasContaining(s.areas, n.Position)
			out.KindOf[n.Name] = n.Kind
		}
		return out, nil
	})

	st.Handle("provisioning.store-readback", func(w *state.World, p any) (any, error) {
		m, ok := p.(map[string]provision.NodeState)
		if !ok {
			return nil, fmt.Errorf("provisioning.store-readback needs a readback")
		}
		s.readback = m
		s.readbackDone = true
		s.readbackAtMs = w.NowMs
		w.ProvisioningReadAt, w.ProvisioningRead = w.NowMs, true
		s.refreshMatches(w)
		return map[string]any{"read": len(m)}, nil
	})

	st.Handle("provisioning.store-results", func(w *state.World, p any) (any, error) {
		results, ok := p.([]state.ProvisionResult)
		if !ok {
			return nil, fmt.Errorf("provisioning.store-results needs results")
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Node < results[j].Node })
		w.ProvisioningResults = results
		refused := 0
		for _, r := range results {
			if len(r.Refused) > 0 {
				refused++
			}
		}
		if refused > 0 {
			w.Say(fmt.Sprintf("provisioned %d nodes; %d refused part of their script",
				len(results), refused))
		} else {
			w.Say(fmt.Sprintf("provisioned %d nodes", len(results)))
		}
		return map[string]any{"nodes": len(results), "refused": refused}, nil
	})
}

// refreshMatches recomputes how many nodes match each rule against the last
// readback, so a rule matching nobody is visible while it is being written -
// not only after a run. Cheap: it is arithmetic over an in-memory map, no
// firmware involved.
func (s *Sim) refreshMatches(w *state.World) {
	rules := s.activeRules()
	w.ProvisioningRules = toWorldRules(rules)
	if !s.readbackDone {
		w.ProvisioningMatch = nil
		return
	}
	counts := make(map[string]int, len(rules))
	for _, ns := range s.readback {
		for _, r := range rules {
			if r.Matches(ns) {
				counts[r.Name]++
			}
		}
	}
	w.ProvisioningMatch = counts
}

func stringSetField(p any, key string) map[string]bool {
	m, ok := p.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out[s] = true
		}
	}
	return out
}

func areasContaining(areas []scenario.Boundary, pos scenario.LatLon) []string {
	byName := map[string][]scenario.Boundary{}
	var order []string
	for _, b := range areas {
		if _, seen := byName[b.Name]; !seen {
			order = append(order, b.Name)
		}
		byName[b.Name] = append(byName[b.Name], b)
	}
	var out []string
	for _, name := range order {
		r := scenario.Region{Boundaries: byName[name]}
		if r.Contains(pos) {
			out = append(out, name)
		}
	}
	return out
}

// settleSteps is how long to wait for a burst of commands to be answered,
// scaled by the longest single node's script rather than the total across the
// network - every node's firmware drains its own queue in parallel, so the
// bound is how many lines the slowest one has to get through, not how many
// nodes there are.
func settleSteps(maxLinesPerNode int) int {
	n := maxLinesPerNode * 12
	if n < 120 {
		n = 120
	}
	if n > 900 {
		n = 900
	}
	return n
}

// isRefusal reports whether a reply is the firmware declining a command,
// against the exact refusal strings CommonCLI.cpp sends: "Error, must be...",
// "Err - ...", "(ERR: ...)", and the CLI's own "Unknown command" for a set
// key it does not recognise.
func isRefusal(reply string) bool {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return false
	}
	for _, marker := range []string{"Err", "ERR", "Unknown command"} {
		if strings.Contains(reply, marker) {
			return true
		}
	}
	return false
}

// transcriptEntry is one command from a burst and whatever the firmware said
// back to it - reconstructed from a flat scrollback on the assumption every
// simulation already makes for a burst of commands: the firmware answers
// them in the order they arrived, one per loop iteration, before the next is
// read.
type transcriptEntry struct {
	Command string
	Reply   []string
}

// splitTranscript walks lines captured since a mark and attributes each
// firmware reply to the command that produced it.
//
// An echo cannot be told apart from a reply by its formatting alone: Echo
// writes "> " before whatever was typed, and several of the firmware's own
// get replies write "> " before their own answer as part of the reply text
// itself (get path.hash.mode answers "> 0"). Two different things produce the
// same-looking line. So this matches by content instead, against the exact
// commands that were sent, in the order they were sent - which this package
// always knows, because it is the one that sent them.
func splitTranscript(lines []string, expected []string) []transcriptEntry {
	out := make([]transcriptEntry, 0, len(expected))
	next := 0
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		if first, rest, ok := strings.Cut(l, " "); ok && looksLikeTimestamp(first) {
			l = strings.TrimSpace(rest)
		}
		if next < len(expected) && l == "> "+expected[next] {
			out = append(out, transcriptEntry{Command: expected[next]})
			next++
			continue
		}
		if len(out) == 0 {
			continue
		}
		out[len(out)-1].Reply = append(out[len(out)-1].Reply, l)
	}
	return out
}

func looksLikeTimestamp(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// runProvisioning is the whole pipeline for one batch of nodes: read what
// they hold, resolve the active rules against it, send only what changes,
// and write the transcript into each node's own console. Run from the
// firmware-starting goroutine or provisioning.apply's own goroutine, never
// from the store's - three hundred nodes times several commands each is work
// proportional to the network, and that does not belong on the thread every
// other verb is waiting behind.
func (s *Sim) runProvisioning(ctx context.Context, st *state.Store, only []string) (sent int) {
	pc, states, ok := s.readNodes(ctx, st, only)
	if !ok {
		return 0
	}

	maxLines := 0
	sendMarks := map[string]int{}
	commands := map[string][]provision.ResolvedCommand{}
	companionSent := map[string]int{}
	for _, n := range pc.Nodes {
		buf, ok := pc.Bufs[n.Name]
		if !ok {
			continue
		}
		cmds := provision.Resolve(pc.Rules, states[n.Name])
		commands[n.Name] = cmds
		en, ok := s.eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}

		if n.Kind == scenario.Companion {
			frames := companionFrames(cmds)
			buf.Section(sectionLine(fmt.Sprintf(
				"provisioning over the app protocol  %d command(s)", len(frames))))
			for _, f := range frames {
				_ = en.Firmware.Bridge.Type(compFrame(f))
			}
			companionSent[n.Name] = len(frames)
			if len(frames) > 0 {
				sent++
			}
			continue
		}

		if len(cmds) > maxLines {
			maxLines = len(cmds)
		}
		buf.Section(sectionLine(fmt.Sprintf("provisioning  %d command(s)", len(cmds))))
		buf.Stamp(fmt.Sprintf("# %s: %s, %s", n.Name, n.Kind, n.Firmware.Version))
		sendMarks[n.Name] = buf.Mark()
		for _, c := range cmds {
			buf.Echo(c.Command)
			_ = en.Firmware.Bridge.Type([]byte(c.Command + "\r\n"))
		}
		if len(cmds) > 0 {
			sent++
		}
	}
	if maxLines > 0 {
		_, _ = st.Do(ctx, "sim.settle", map[string]any{"steps": settleSteps(maxLines)})
	}

	results := make([]state.ProvisionResult, 0, len(pc.Nodes))
	for _, n := range pc.Nodes {
		buf, ok := pc.Bufs[n.Name]
		if !ok {
			continue
		}
		if n.Kind == scenario.Companion {
			// No reply to classify - the app protocol answers with framed
			// binary, not a line this transcript can show as accepted or
			// refused, so the count is what was sent and nothing more is
			// claimed about what the companion did with it.
			results = append(results, state.ProvisionResult{
				Node: n.Name, Sent: companionSent[n.Name],
			})
			continue
		}
		cmds := commands[n.Name]
		sentCmds := make([]string, len(cmds))
		for i, c := range cmds {
			sentCmds[i] = c.Command
		}
		lines := buf.LinesSince(sendMarks[n.Name])
		entries := splitTranscript(lines, sentCmds)
		var refused []string
		for _, e := range entries {
			reply := strings.Join(e.Reply, " ")
			if isRefusal(reply) {
				refused = append(refused, e.Command+": "+reply)
			}
		}
		accepted := len(cmds) - len(refused)
		if accepted < 0 {
			accepted = 0
		}
		buf.Section(sectionLine(fmt.Sprintf("%d sent, %d accepted", len(cmds), accepted)))
		results = append(results, state.ProvisionResult{
			Node: n.Name, Sent: len(cmds), Refused: refused,
		})
	}
	_, _ = st.Do(ctx, "provisioning.store-results", results)
	return sent
}

// runReadbackOnly is the read phase alone: asking every target what it holds,
// without deciding or sending anything - "read now", for seeing what a mesh
// currently is rather than changing it.
func (s *Sim) runReadbackOnly(st *state.Store, only []string) {
	s.readNodes(context.Background(), st, only)
}

// readNodes is the shared read phase: attach, ask, settle, parse, and record
// what came back. Returns false if there was nothing to read.
func (s *Sim) readNodes(ctx context.Context, st *state.Store, only []string,
) (provisioningContext, map[string]provision.NodeState, bool) {
	var params any
	if only != nil {
		names := make([]any, len(only))
		for i, n := range only {
			names[i] = n
		}
		params = map[string]any{"nodes": names}
	}
	res, err := st.Do(ctx, "provisioning.context", params)
	if err != nil {
		return provisioningContext{}, nil, false
	}
	pc, ok := res.(provisioningContext)
	if !ok || len(pc.Nodes) == 0 {
		return provisioningContext{}, nil, false
	}

	// A companion is not part of this: companion_radio answers no `get`, no
	// `region` - it is not built on CommonCLI at all, and typing text at its
	// binary protocol writes garbage into a stream that frames on byte
	// length rather than newlines. See provisioningcompanion.go.
	reads := provision.RequiredReads(pc.Rules)
	marks := map[string]int{}
	for name, buf := range pc.Bufs {
		if pc.KindOf[name] == scenario.Companion {
			continue
		}
		marks[name] = buf.Mark()
		en, ok := s.eng.NodeByName(name)
		if !ok || en.Firmware == nil {
			continue
		}
		for _, cmd := range reads {
			buf.Echo(cmd)
			_ = en.Firmware.Bridge.Type([]byte(cmd + "\r\n"))
		}
	}
	_, _ = st.Do(ctx, "sim.settle", map[string]any{"steps": settleSteps(len(reads))})

	states := make(map[string]provision.NodeState, len(pc.Nodes))
	for _, n := range pc.Nodes {
		if n.Kind == scenario.Companion {
			// No CLI transcript to read; the base's own effects - name,
			// position, path hash, clock, default scope - still resolve,
			// because a rule with no conditions needs no readback to match.
			states[n.Name] = parseReadback(n, nil, pc.Areas[n.Name], pc.Selected[n.Name])
			continue
		}
		buf, ok := pc.Bufs[n.Name]
		if !ok {
			continue
		}
		states[n.Name] = parseReadback(n, splitTranscript(buf.LinesSince(marks[n.Name]), reads),
			pc.Areas[n.Name], pc.Selected[n.Name])
	}
	_, _ = st.Do(ctx, "provisioning.store-readback", states)
	return pc, states, true
}

// sectionLine is the divider the transcript uses to mark where a
// provisioning run starts and how it concluded - deliberately unlike
// anything the firmware itself would print, so it reads as an annotation
// rather than another line of output.
func sectionLine(middle string) string {
	const width = 60
	label := "  " + middle + "  "
	dashes := width - len(label)
	if dashes < 4 {
		dashes = 4
	}
	return "──" + label + strings.Repeat("─", dashes)
}

// parseReadback turns the reply lines a batch of get-commands produced into a
// NodeState. Missing or unparsable answers are left unset rather than
// defaulted - a firmware too old to answer a field must not be mistaken for
// one that answered zero or off.
func parseReadback(n scenario.Node, entries []transcriptEntry, areas []string, selected bool) provision.NodeState {
	ns := provision.NodeState{
		Name: n.Name, Kind: string(n.Kind), Lat: n.Position.Lat, Lon: n.Position.Lon,
		Selected: selected, Areas: areas, Companion: n.Kind == scenario.Companion,
		Read: true, Values: map[string]string{},
		Imported: provision.ImportedFacts{
			Name: n.Name, Lat: n.Position.Lat, Lon: n.Position.Lon,
			Regions: n.Regions, DefaultScope: n.DefaultScope, AllowAnyFlood: n.AllowAnyFlood,
		},
	}
	for _, e := range entries {
		reply := strings.Join(e.Reply, " ")
		switch e.Command {
		case "region":
			ns.Regions, ns.UnscopedFlood = provision.ParseRegionTree(e.Reply)
		case "region default":
			ns.DefaultScope, ns.DefaultScopeKnown = provision.ParseRegionDefault(reply)
		default:
			// Which Key this answers, not merely whether the text happens to
			// start with "get " - a custom condition's own get line looks
			// exactly like a table one syntactically, and only the table says
			// which key a command actually belongs to.
			key := "custom:" + e.Command
			if name, ok := keyByGet[e.Command]; ok {
				key = name
			}
			if v, ok := provision.ParseGetReply(reply); ok {
				ns.Values[key] = v
			}
		}
	}
	return ns
}

// keyByGet maps a Get command back to the Key.Name that owns it, built once -
// the reverse of provision.ByName, needed because a readback only has the
// command string a reply answered, not which key asked for it.
var keyByGet = func() map[string]string {
	m := make(map[string]string, len(provision.Table))
	for _, k := range provision.Table {
		if k.Get != "" {
			m[k.Get] = k.Name
		}
	}
	return m
}()
