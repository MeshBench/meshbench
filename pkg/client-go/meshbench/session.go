// The rest of a scripted run: the project, what happened, and what a node
// said.
package meshbench

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Project is opening, saving and starting over. Live.
type Project struct{ w *Workbench }

// Project reaches the network as a whole.
func (w *Workbench) Project() Project { return Project{w} }

// New is an empty network.
//
// With a place, it becomes the study area and the map is framed on it, because
// those are the same wish - and because a blank network with no place is a map
// in the middle of the Atlantic.
func (p Project) New(ctx context.Context, place string) error {
	params := map[string]any{}
	if place != "" {
		params["place"] = place
	}
	return p.w.Do(ctx, "project.new", params)
}

// Open loads a fixture or a saved project.
func (p Project) Open(ctx context.Context, path string) error {
	return p.w.Do(ctx, "project.open", path)
}

// Save writes the current one out. Worth doing before anything that might
// restart the process: the scenario lives in the process, not on disk.
func (p Project) Save(ctx context.Context, name string) (string, error) {
	var out struct {
		Path string `json:"path"`
	}
	return out.Path, p.w.CallInto(ctx, "project.save", map[string]any{"name": name}, &out)
}

// List is what has been saved.
func (p Project) List(ctx context.Context) ([]string, error) {
	var out struct {
		Projects []string `json:"projects"`
	}
	return out.Projects, p.w.CallInto(ctx, "project.list", nil, &out)
}

// Events is what the engine has done. Live.
type Events struct{ w *Workbench }

// Events reaches the log.
func (w *Workbench) Events() Events { return Events{w} }

// Recent is the tail.
//
// A tail, and only a tail: the store keeps a bounded one because a long run
// has millions, so a script that needs all of them dumps per round rather than
// polling this. Reading only the tail after a busy flood samples the most
// congested moment of it, which is a mistake already made once here.
func (e Events) Recent(ctx context.Context, limit int) ([]Event, error) {
	var out struct {
		Events []Event `json:"events"`
		Total  int     `json:"total"`
	}
	p := map[string]any{}
	if limit > 0 {
		p["limit"] = float64(limit)
	}
	return out.Events, e.w.CallInto(ctx, "events.recent", p, &out)
}

// Total is how many there have been, which is the cheap question.
func (e Events) Total(ctx context.Context) (int, error) {
	var out struct {
		Total int `json:"total"`
	}
	return out.Total, e.w.CallInto(ctx, "events.recent", map[string]any{"limit": 1.0}, &out)
}

// Dump writes every event held to a file, one JSON object per line.
func (e Events) Dump(ctx context.Context, path string) (int, error) {
	var out struct {
		Written int `json:"written"`
	}
	return out.Written, e.w.CallInto(ctx, "events.dump", map[string]any{"path": path}, &out)
}

// Match is what an event has to be for Wait to stop.
//
// Empty fields match anything, so waiting for "any reception at Glenrothes" is
// Match{Kind: "rx", To: "Glenrothes"} and not a predicate somebody has to
// write.
type Match struct {
	Kind string
	From string
	To   string
}

func (m Match) matches(e Event) bool {
	return (m.Kind == "" || e.Kind == m.Kind) &&
		(m.From == "" || e.From == m.From) &&
		(m.To == "" || e.To == m.To)
}

// Wait blocks until an event matches, and returns it.
func (e Events) Wait(ctx context.Context, m Match, timeout time.Duration) (Event, error) {
	var found Event
	err := waitFor(ctx, timeout, "an event matching "+m.describe(),
		func() (bool, string, error) {
			evs, err := e.Recent(ctx, 500)
			if err != nil {
				return false, "", err
			}
			for _, ev := range evs {
				if m.matches(ev) {
					found = ev
					return true, "", nil
				}
			}
			return false, fmt.Sprintf("%d events, none matching", len(evs)), nil
		})
	return found, err
}

func (m Match) describe() string {
	var parts []string
	if m.Kind != "" {
		parts = append(parts, m.Kind)
	}
	if m.From != "" {
		parts = append(parts, "from "+m.From)
	}
	if m.To != "" {
		parts = append(parts, "to "+m.To)
	}
	if len(parts) == 0 {
		return "anything"
	}
	return strings.Join(parts, " ")
}

// Console is one node's firmware console. Live.
type Console struct {
	w    *Workbench
	node string
}

// Console reaches a node's console.
func (n Node) Console() Console { return Console{w: n.w, node: n.name} }

// Send types a line at it.
//
// Which verb that is depends on what the node runs. A companion or a room
// server speaks the framed companion protocol, so its console takes a command
// rather than keystrokes, and console.type reaches a node of that kind without
// ever being delivered.
func (c Console) Send(ctx context.Context, line string) error {
	return c.w.Do(ctx, c.verb(ctx),
		map[string]any{"node": c.node, "command": line})
}

// verb is console.cli for a framed console and console.type for a typed one.
//
// A node this client cannot see is not one to guess about, so the typed verb
// is the fallback and the refusal it gives says so in its own words.
func (c Console) verb(ctx context.Context) string {
	info, err := c.w.Nodes().Get(ctx, c.node)
	if err != nil {
		return "console.type"
	}
	return consoleVerb(info.Kind)
}

// consoleVerb is the mapping itself, apart from the lookup, so it can be held
// against the kinds this build knows about rather than only against a running
// workbench.
//
// A companion and a room server speak the framed companion protocol: their
// console takes a command, not keystrokes. Everything else has a serial port
// somebody types at. The Python client has always drawn the line here and this
// one did not, so a companion driven from Go was sent keystrokes it never read.
func consoleVerb(k Kind) string {
	switch k {
	case Companion, RoomServer:
		return "console.cli"
	}
	return "console.type"
}

// Read is the scrollback.
// Read is the scrollback, newest last.
//
// The lines come back under "tail" and "lines" is how many there are in total,
// so reading "lines" hands you a number where you asked for text. The tail is
// the last 200; a node up for an hour has thousands and nobody reads the first
// one.
func (c Console) Read(ctx context.Context) ([]string, error) {
	var out struct {
		Tail  []string `json:"tail"`
		Lines int      `json:"lines"`
	}
	err := c.w.CallInto(ctx, "console.read", map[string]any{"node": c.node}, &out)
	return out.Tail, err
}

// Ask sends a line and waits for the node to answer it.
//
// The important one. A node reads its serial input on its next loop and its
// loop only runs when the engine steps, so reading straight after sending
// reads the moment *before* the command was sent - every script that has done
// this by hand got an empty reply and concluded the console was broken. This
// gives the mesh its own time first, by stepping when the run is paused.
func (c Console) Ask(ctx context.Context, line string, steps int) (string, error) {
	before, err := c.Read(ctx)
	if err != nil {
		return "", err
	}
	if err := c.Send(ctx, line); err != nil {
		return "", err
	}
	if steps <= 0 {
		steps = 100
	}
	st, err := c.w.Sim().State(ctx)
	if err != nil {
		return "", err
	}
	if st.Playing {
		// Already moving, so it will be answered on its own; give it the same
		// amount of the mesh's time a settle would.
		if err := c.w.Sim().WaitUntil(ctx,
			st.NowMs+uint32(steps)*st.StepMs, time.Minute); err != nil {
			return "", err
		}
	} else if err := c.w.Sim().Settle(ctx, steps); err != nil {
		return "", err
	}
	after, err := c.Read(ctx)
	if err != nil {
		return "", err
	}
	if len(after) <= len(before) {
		return "", nil
	}
	return strings.Join(after[len(before):], "\n"), nil
}

// Provenance is what this session's measurements are being made under.
//
// Read from the snapshot rather than carried on each result, for now: the
// verbs do not return it yet, and inventing it client-side would be a claim
// this client is not entitled to make. What it does guarantee is that the
// numbers here are the session's actual settings at the moment of asking.
func (w *Workbench) Provenance(ctx context.Context) (Provenance, error) {
	snap, err := w.Snapshot(ctx)
	if err != nil {
		return Provenance{}, err
	}
	p := Provenance{RFMode: str(snap["rf_mode"])}
	if v, ok := snap["excess_loss_db"].(float64); ok {
		p.ExcessLossDB = v
	}
	p.Calibrated = snap["calibrated"] == true
	if v, ok := snap["seed"].(float64); ok {
		p.Seed = uint64(v)
	}
	return p, nil
}
