// Scheduled sends and assertions.
//
// A fixture carries these, and the old workbench could edit them: add a send,
// set an assertion, run a stress ramp, snapshot a baseline. The Gio build drew
// them as two read-only tables, which is enough to see that a fixture has one
// assertion and no way at all to add a second.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerSchedule(st *state.Store, s *Sim) {
	st.Handle("schedule.add", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		if node == "" {
			return nil, fmt.Errorf("schedule.add needs a node")
		}
		if _, ok := findNode(w.Nodes, node); !ok {
			return nil, noSuchNode(node)
		}
		snd := state.Send{Node: node}
		if v, ok := namedNum(p, "at_ms"); ok {
			snd.AtMs = uint32(v)
		}
		if v, ok := namedNum(p, "every_ms"); ok {
			snd.EveryMs = uint32(v)
		}
		if c, ok := namedField(p, "command"); ok {
			snd.Command = c
		}
		w.Sends = append(w.Sends, snd)
		w.Say(fmt.Sprintf("%s sends at %.1f s", node, float64(snd.AtMs)/1000))
		return map[string]any{"sends": len(w.Sends)}, nil
	})

	st.Handle("schedule.clear", func(w *state.World, _ any) (any, error) {
		n := len(w.Sends)
		w.Sends = nil
		w.Say(fmt.Sprintf("cleared %d scheduled sends", n))
		return map[string]any{"cleared": n}, nil
	})

	st.Handle("assert.add", func(w *state.World, p any) (any, error) {
		kind, _ := stringField(p, "kind")
		if kind == "" {
			return nil, fmt.Errorf("assert.add needs a kind")
		}
		a := state.Assertion{Kind: kind}
		a.Node, _ = namedField(p, "node")
		if v, ok := namedNum(p, "at_least"); ok {
			a.AtLeast = int(v)
		}
		if v, ok := namedNum(p, "at_most"); ok {
			a.AtMost = int(v)
		}
		if v, ok := namedNum(p, "max_pct"); ok {
			a.MaxPct = v
		}
		if v, ok := namedNum(p, "within_ms"); ok {
			a.WithinMs = uint32(v)
		}
		w.Assertions = append(w.Assertions, a)
		w.Say("assertion added: " + kind)
		return map[string]any{"assertions": len(w.Assertions)}, nil
	})

	// Reported per assertion rather than as one verdict, because "it failed"
	// without which one is a starting point rather than an answer.
	st.Handle("assert.check", func(w *state.World, _ any) (any, error) {
		if len(w.Assertions) == 0 {
			return nil, fmt.Errorf("this scenario carries no assertions")
		}
		results := make([]map[string]any, 0, len(w.Assertions))
		passed := 0
		for _, a := range w.Assertions {
			ok, got, want := checkAssertion(w, a)
			if ok {
				passed++
			}
			results = append(results, map[string]any{
				"kind": a.Kind, "node": a.Node, "pass": ok,
				"got": got, "want": want,
			})
		}
		w.Say(fmt.Sprintf("%d of %d assertions pass", passed, len(w.Assertions)))
		return map[string]any{
			"passed": passed, "total": len(w.Assertions), "results": results,
		}, nil
	})
}

// checkAssertion measures one claim against the run so far.
func checkAssertion(w *state.World, a state.Assertion) (ok bool, got, want string) {
	switch strings.ToLower(a.Kind) {
	case "delivered", "deliveries", "unique_deliveries":
		seen := map[string]bool{}
		for _, e := range w.Events {
			if e.Kind == "rx" {
				seen[e.To] = true
			}
		}
		n := len(seen)
		want = fmt.Sprintf("at least %d", a.AtLeast)
		return n >= a.AtLeast, fmt.Sprintf("%d", n), want
	case "sent", "transmissions":
		n := 0
		for _, e := range w.Events {
			if e.Kind == "tx" && (a.Node == "" || e.From == a.Node) {
				n++
			}
		}
		if a.AtMost > 0 {
			return n <= a.AtMost, fmt.Sprintf("%d", n), fmt.Sprintf("at most %d", a.AtMost)
		}
		return n >= a.AtLeast, fmt.Sprintf("%d", n), fmt.Sprintf("at least %d", a.AtLeast)
	}
	// An assertion whose kind is not understood must not quietly pass: that
	// is a green run that checked nothing.
	return false, "not measured", "a kind this build understands"
}
