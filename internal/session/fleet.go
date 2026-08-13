// Sending the same thing to every node.
//
// A mesh is configured by typing the same line at forty repeaters, and doing
// that by hand is where the region spelling trap was paid for twice. The old
// workbench had a Fleet window; this build had the panel and no way to send.
package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
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
			replies[name] = strings.Join(buf.LinesSince(mark), " ")
		}
		w.Say(fmt.Sprintf("sent %q to %d nodes", cmd, len(targets)))
		out := map[string]any{
			"command": cmd, "sent_to": len(targets), "replies": replies,
		}
		if warn != "" {
			out["warning"] = warn
		}
		return out, nil
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
