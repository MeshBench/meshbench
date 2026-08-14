// Sending the same thing to every node.
//
// A mesh is configured by typing the same line at forty repeaters, and doing
// that by hand is where the region spelling trap was paid for twice. The old
// workbench had a Fleet window; this build had the panel and no way to send.
package session

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func registerFleet(st *state.Store, s *Sim) {
	// fleet.send: one command, to every node or to a filtered subset.
	//
	// Replies are collected per node rather than merged, because the answer
	// worth having is which node disagreed with the others.
	st.Handle("fleet.send", func(w *state.World, p any) (any, error) {
		cmd, _ := stringField(p, "command")
		if strings.TrimSpace(cmd) == "" {
			return nil, fmt.Errorf("fleet.send needs a command")
		}
		only, _ := stringField(p, "node")
		kind, _ := stringField(p, "kind")

		targets := make([]string, 0, len(s.nodes))
		for _, n := range s.nodes {
			if only != "" && n.Name != only {
				continue
			}
			if kind != "" && string(n.Kind) != kind {
				continue
			}
			if en, ok := s.eng.NodeByName(n.Name); !ok || en.Firmware == nil {
				continue
			}
			targets = append(targets, n.Name)
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("no node is running firmware, so there is nothing to send to")
		}
		sort.Strings(targets)

		// Which commands invalidate a run, said before sending rather than
		// after the numbers look strange.
		warn := ""
		if invalidates(cmd) {
			warn = "this changes what the nodes are, so anything already " +
				"measured is a different mesh"
		}
		replies := map[string]string{}
		marks := map[string]int{}
		for _, name := range targets {
			buf, err := s.consoleFor(name)
			if err != nil {
				replies[name] = "no console: " + err.Error()
				continue
			}
			mark := buf.Mark()
			buf.Echo(cmd)
			en, _ := s.eng.NodeByName(name)
			if err := en.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
				replies[name] = "send failed: " + err.Error()
				continue
			}
			marks[name] = mark
			replies[name] = ""
		}
		// The replies do not exist yet.
		//
		// A node answers on its own next loop, and its loop only runs when the
		// engine steps. Reading the console here read the moment before the
		// command was sent, so every reply was empty and the panel had nothing
		// to show - which is what "fleet commands do not work" looked like.
		// The old workbench steps a second of simulated time between sending
		// and reading; this cannot do that here, because stepping is the
		// store's own work and this is the store's own goroutine.
		s.fleetPending = &fleetPending{cmd: cmd, marks: marks}
		w.FleetCommand, w.FleetReplies = cmd, nil
		w.Say(fmt.Sprintf("sent %q to %d nodes; waiting for their replies", cmd, len(targets)))
		go s.collectFleet(st, w.Playing)
		out := map[string]any{
			"command": cmd, "sent_to": len(targets), "replies": replies,
		}
		if warn != "" {
			out["warning"] = warn
		}
		return out, nil
	})

	// fleet.replies: what came back, once the engine has run far enough for
	// the nodes to have answered.
	st.Handle("fleet.replies", func(w *state.World, _ any) (any, error) {
		pend := s.fleetPending
		if pend == nil {
			return map[string]any{"replies": 0}, nil
		}
		s.fleetPending = nil
		out := make([]state.FleetReply, 0, len(pend.marks))
		for name, mark := range pend.marks {
			buf, err := s.consoleFor(name)
			if err != nil {
				out = append(out, state.FleetReply{Node: name, Reply: "no console"})
				continue
			}
			said := answerOnly(buf.LinesSince(mark), cmdOf(pend))
			// A companion hears the line and does not answer it.
			//
			// It speaks the binary protocol a phone speaks, not the repeater
			// CLI, so a fleet command reaches it and means nothing - and a
			// blank row reads as a node that failed rather than one that was
			// asked the wrong question.
			if isCompanionNode(s.nodes, name) {
				said = "a companion: speaks the app protocol, not this CLI - " +
					"use its own tab"
			} else if said == "" {
				said = "-"
			}
			out = append(out, state.FleetReply{Node: name, Reply: said})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
		w.FleetReplies, w.FleetCommand = out, pend.cmd
		w.Say(fmt.Sprintf("%d nodes answered %q", len(out), pend.cmd))
		return map[string]any{"replies": len(out)}, nil
	})

	// nodes.regions and nodes.allow_flood: the two that decide whether
	// anything relays at all.
	st.Handle("nodes.regions", func(w *state.World, p any) (any, error) {
		var regions []string
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["regions"].([]any); ok {
				for _, x := range xs {
					if str, ok := x.(string); ok {
						regions = append(regions, str)
					}
				}
			}
		}
		only, _ := stringField(p, "node")
		n := 0
		for i := range s.nodes {
			if only != "" && s.nodes[i].Name != only {
				continue
			}
			s.nodes[i].Regions = append([]string(nil), regions...)
			n++
		}
		for i := range w.Nodes {
			if only == "" || w.Nodes[i].Name == only {
				w.Nodes[i].Regions = append([]string(nil), regions...)
			}
		}
		w.Say(fmt.Sprintf("%d nodes now hold %s", n, strings.Join(regions, " ")))
		return map[string]any{"nodes": n, "regions": regions}, nil
	})

	st.Handle("nodes.allow_flood", func(w *state.World, p any) (any, error) {
		on := true
		if v, ok := boolField(p, "on"); ok {
			on = v
		}
		only, _ := stringField(p, "node")
		n := 0
		for i := range s.nodes {
			if only != "" && s.nodes[i].Name != only {
				continue
			}
			s.nodes[i].AllowAnyFlood = on
			n++
		}
		// The wildcard is the parent of every region, so a flood is forwarded
		// whatever its scope. It is also the difference between a fixture that
		// relays and one that transmits everything, relays nothing, and
		// reports no error at all.
		w.Say(fmt.Sprintf("%d nodes now %s any flood", n,
			map[bool]string{true: "allow", false: "refuse"}[on]))
		return map[string]any{"nodes": n, "allow_any_flood": on}, nil
	})

	// nodes.delete: the destructive one, so it says what it removed.
	st.Handle("nodes.delete", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, fmt.Errorf("nodes.delete needs a node")
		}
		kept := make([]scenario.Node, 0, len(s.nodes))
		for _, n := range s.nodes {
			if n.Name != name {
				kept = append(kept, n)
			}
		}
		if len(kept) == len(s.nodes) {
			return nil, fmt.Errorf("no node named %q", name)
		}
		s.buildSeeded(kept, s.freqMHz, s.seed)
		out := w.Nodes[:0]
		for _, n := range w.Nodes {
			if n.Name != name {
				out = append(out, n)
			}
		}
		w.Nodes = out
		w.Links = s.links()
		w.Say("deleted " + name)
		return map[string]any{"deleted": name, "nodes": len(kept)}, nil
	})
}

// invalidates reports whether a command changes what the nodes are, rather
// than only asking them something.
//
// Said before sending, because the alternative is discovering it afterwards
// from numbers that look plausible: a region added halfway through a run makes
// the second half a different mesh from the first.
func invalidates(cmd string) bool {
	head := strings.ToLower(strings.Fields(strings.TrimSpace(cmd))[0])
	switch head {
	case "region", "set", "reboot", "clock", "advert", "erase", "reset":
		return true
	}
	return false
}

// fleetPending is a command sent and not yet answered.
type fleetPending struct {
	cmd   string
	marks map[string]int
}

// collectFleet gives the mesh a second of its own time to answer, then asks
// for the replies.
//
// A second, because a node reads its serial input on its next loop and answers
// within a tick or two - the same window the old workbench uses. When the run
// is already playing the store's ticker provides that time, so this only waits
// for it; when it is paused nothing would ever step, so the steps are asked
// for one at a time through the store rather than taken from under it.
func (s *Sim) collectFleet(st *state.Store, playing bool) {
	ctx := context.Background()
	if playing {
		// Two seconds of the mesh's own time, not the wall's.
		//
		// A wall-clock wait was right on the small fixture and wrong on the
		// big one: 311 real firmware processes make a simulated second cost
		// many wall seconds, so the wait expired before the nodes' loops had
		// run and every repeater "said" nothing. What the reply actually
		// needs is simulated time, so that is what is waited for - with a
		// wall-clock ceiling so a paused or wedged run still answers with
		// what it has.
		start := uint32(0)
		if snap := st.Snapshot(); snap != nil {
			start = snap.NowMs
		}
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(300 * time.Millisecond)
			snap := st.Snapshot()
			if snap == nil || snap.NowMs >= start+2000 || !snap.Playing {
				break
			}
		}
	} else {
		for i := 0; i < 200; i++ {
			if _, err := st.Do(ctx, "sim.step", nil); err != nil {
				break
			}
		}
	}
	_, _ = st.Do(ctx, "fleet.replies", nil)
}

// cmdOf is the command a pending fleet send was about.
func cmdOf(p *fleetPending) string { return p.cmd }

// answerOnly is what the node said, without the conversation around it.
//
// A console line carries the simulated time it was written at, and the node
// echoes what it was sent before answering it. Grouped replies made that
// obvious: every row read "5.010 > advert 5.010 advert 5.010 -> OK", where the
// only part anybody wants is the last few words.
func answerOnly(lines []string, cmd string) string {
	var out []string
	for _, l := range lines {
		// Drop the timestamp, which is the first field and a number. Trimmed
		// first: the line is padded on the left, so looking for the gap after
		// the timestamp found the padding instead and changed nothing.
		l = strings.TrimSpace(l)
		if first, rest, ok := strings.Cut(l, " "); ok {
			if _, err := strconv.ParseFloat(first, 64); err == nil {
				l = strings.TrimSpace(rest)
			}
		}
		l = strings.TrimPrefix(l, "-> ")
		switch {
		case l == "", l == cmd, l == "> "+cmd:
		default:
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, " "))
}
