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
	st.HandleSpec("schedule.add", state.Spec{
		What: "have a node originate traffic on its own at a stated moment of " +
			"simulated time, so a run reproduces without anybody watching it " +
			"for the moment to press send",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node that sends; absent is refused, and so is a name " +
					"no node in this network carries"},
			{Name: "at_ms", Type: state.ParamNumber,
				What: "the simulated instant of the first send; absent is zero, " +
					"which is the start of the run"},
			{Name: "every_ms", Type: state.ParamNumber,
				What: "repeat interval in simulated milliseconds; absent or zero " +
					"sends once"},
			{Name: "command", Type: state.ParamString,
				What: "what the node is told to do, in its own console's words; " +
					"has to be named rather than passed bare, and absent leaves " +
					"it empty"},
		},
		Returns: []string{"sends"},
		Answers: "`sends` is how many the schedule now holds, not the one just " +
			"added.",
		Example: &state.Example{
			Params:   map[string]any{"node": "West Lomond", "at_ms": 30000.0},
			What:     "one flood thirty seconds into the run",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		if node == "" {
			return nil, fmt.Errorf("schedule.add needs a node")
		}
		if _, ok := findNode(w.Nodes, node); !ok {
			return nil, noSuchNode(node)
		}
		snd := state.Send{Node: node}
		if v, ok := numField(p, "at_ms"); ok {
			snd.AtMs = uint32(v)
		}
		if v, ok := numField(p, "every_ms"); ok {
			snd.EveryMs = uint32(v)
		}
		if c, ok := namedField(p, "command"); ok {
			snd.Command = c
		}
		w.Sends = append(w.Sends, snd)
		w.Say(fmt.Sprintf("%s sends at %.1f s", node, float64(snd.AtMs)/1000))
		return map[string]any{"sends": len(w.Sends)}, nil
	})

	st.HandleSpec("schedule.clear", state.Spec{
		What: "empty the schedule, which is what editing it amounts to: nothing " +
			"removes a single send, so a schedule is rebuilt rather than amended",
		Returns: []string{"cleared"},
		Example: &state.Example{
			Params: map[string]any{}, What: "start the schedule again", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		n := len(w.Sends)
		w.Sends = nil
		w.Say(fmt.Sprintf("cleared %d scheduled sends", n))
		return map[string]any{"cleared": n}, nil
	})

	st.HandleSpec("assert.add", state.Spec{
		What: "state what has to be true for a run to have passed, so a scripted " +
			"run ends with a verdict rather than a table somebody has to read",
		Params: []state.Param{
			{Name: "kind", Type: state.ParamString, Required: true, Primary: true,
				What: "what is measured: `delivered` (or `deliveries`, or " +
					"`unique_deliveries`) counts the nodes that heard anything, " +
					"`sent` (or `transmissions`) counts transmissions; absent is " +
					"refused, and a kind this build does not understand is " +
					"accepted here and fails at the check rather than passing"},
			{Name: "node", Type: state.ParamString,
				What: "restrict a transmission count to one node; has to be " +
					"named, and absent counts every node"},
			{Name: "at_least", Type: state.ParamNumber,
				What: "the floor the count has to reach; absent is zero, which " +
					"any count clears"},
			{Name: "at_most", Type: state.ParamNumber,
				What: "the ceiling, which a transmission count uses in place of " +
					"at_least whenever it is above zero"},
			{Name: "max_pct", Type: state.ParamNumber,
				What: "recorded on the assertion and carried in the fixture, and " +
					"read by no kind this build checks"},
			{Name: "within_ms", Type: state.ParamNumber,
				What: "recorded on the assertion and carried in the fixture, and " +
					"read by no kind this build checks"},
		},
		Returns: []string{"assertions"},
		Answers: "`assertions` is how many the scenario now carries, not the one " +
			"just added.",
		Example: &state.Example{
			Params:   map[string]any{"kind": "delivered", "at_least": 2.0},
			What:     "the flood has to reach at least two nodes",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		kind, _ := stringField(p, "kind")
		if kind == "" {
			return nil, fmt.Errorf("assert.add needs a kind")
		}
		a := state.Assertion{Kind: kind}
		a.Node, _ = namedField(p, "node")
		if v, ok := numField(p, "at_least"); ok {
			a.AtLeast = int(v)
		}
		if v, ok := numField(p, "at_most"); ok {
			a.AtMost = int(v)
		}
		if v, ok := numField(p, "max_pct"); ok {
			a.MaxPct = v
		}
		if v, ok := numField(p, "within_ms"); ok {
			a.WithinMs = uint32(v)
		}
		w.Assertions = append(w.Assertions, a)
		w.Say("assertion added: " + kind)
		return map[string]any{"assertions": len(w.Assertions)}, nil
	})

	// Reported per assertion rather than as one verdict, because "it failed"
	// without which one is a starting point rather than an answer.
	st.HandleSpec("assert.check", state.Spec{
		What: "measure every assertion against the run so far and report each " +
			"one separately, which is what a scripted run exits on",
		Returns: []string{"passed", "total", "results"},
		Answers: "`results` is one row per assertion, carrying `kind`, `node`, " +
			"`pass`, `got` and `want`. A scenario carrying no assertions is " +
			"refused rather than answered with a pass, and an assertion whose " +
			"kind this build does not understand fails with `got` reading " +
			"\"not measured\": a green run that checked nothing is the failure " +
			"this guards against.",
		Example: &state.Example{
			Params: map[string]any{}, What: "did this run pass", Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
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
