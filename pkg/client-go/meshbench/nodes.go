// The network: what is in it, and what one node can be asked to do.
package meshbench

import (
	"context"
	"fmt"
	"time"
)

// Nodes is the collection. Live: every call reads the session.
type Nodes struct{ w *Workbench }

// Nodes reaches the network.
func (w *Workbench) Nodes() Nodes { return Nodes{w} }

// List is every node, as the network currently has them.
func (n Nodes) List(ctx context.Context) ([]NodeInfo, error) {
	var out struct {
		Nodes []NodeInfo `json:"nodes"`
	}
	return out.Nodes, n.w.CallInto(ctx, "nodes.list", nil, &out)
}

// Get is one by name.
func (n Nodes) Get(ctx context.Context, name string) (NodeInfo, error) {
	all, err := n.List(ctx)
	if err != nil {
		return NodeInfo{}, err
	}
	for _, x := range all {
		if x.Name == name {
			return x, nil
		}
	}
	return NodeInfo{}, &Refused{Verb: "nodes.list", Code: "not_found",
		Message: fmt.Sprintf("no node named %q", name), kind: ErrNotFound}
}

// Search finds nodes by name, best first, when you cannot type the name.
//
// Imported names carry emoji and accents - "\U0001F3D4\uFE0F West Lomond \U0001F4E1" is one
// real node - so matching is done on letters and digits alone, with accents
// folded and word order ignored. The ranking happens at the workbench rather
// than here, so this client and the Python one agree about which result is the
// top one.
//
// An empty result is not an error: "nothing matched" is an answer, and the
// caller usually wants to widen the query rather than handle a refusal.
func (n Nodes) Search(ctx context.Context, query string, limit int) ([]NameMatch, error) {
	params := map[string]any{"query": query}
	if limit > 0 {
		params["limit"] = limit
	}
	var out struct {
		Matches []NameMatch `json:"matches"`
	}
	return out.Matches, n.w.CallInto(ctx, "nodes.search", params, &out)
}

// FindLeast is the score below which Find will not act on a top answer.
//
// Taking the top result unconditionally is how a script ends up sending an
// advert from a node that merely shared a word with what was asked for, and it
// does that silently.
const FindLeast = 0.5

// Find is the one node a search meant, or a refusal naming what it did find.
func (n Nodes) Find(ctx context.Context, query string) (Node, error) {
	matches, err := n.Search(ctx, query, 5)
	if err != nil {
		return Node{}, err
	}
	if len(matches) == 0 || matches[0].Score < FindLeast {
		msg := fmt.Sprintf("nothing matches %q well enough", query)
		if len(matches) > 0 {
			near := ""
			for i, m := range matches {
				if i == 3 {
					break
				}
				if i > 0 {
					near += ", "
				}
				near += fmt.Sprintf("%q (%.2f)", m.Name, m.Score)
			}
			msg += "; nearest were " + near
		}
		return Node{}, &Refused{Verb: "nodes.search", Code: "not_found",
			Message: msg, kind: ErrNotFound}
	}
	return n.w.Node(matches[0].Name), nil
}

// Near is the nodes closest to this one, nearest first, at most count of them
// (all of them when count is zero).
//
// Trimming an imported deployment to a neighbourhood is the first thing anybody
// does with one, and the distance is the workbench's own - the same great
// circle its path losses use.
func (n Nodes) Near(ctx context.Context, name string, count int) ([]Neighbour, error) {
	var out struct {
		Near []Neighbour `json:"near"`
	}
	return out.Near, n.w.CallInto(ctx, "nodes.near",
		map[string]any{"node": name, "count": count}, &out)
}

// OfKind filters. Evaluated here rather than at the workbench: it is a
// question about a list somebody already has.
func (n Nodes) OfKind(ctx context.Context, kind Kind) ([]NodeInfo, error) {
	all, err := n.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []NodeInfo
	for _, x := range all {
		if x.Kind == kind {
			out = append(out, x)
		}
	}
	return out, nil
}

// Placement is a node to put down.
type Placement struct {
	Name     string
	Kind     Kind
	Lat, Lon float64
	// HeightM and TxDBm default to the scenario's own defaults when zero -
	// ten metres and 22 dBm - rather than to nothing.
	HeightM float64
	TxDBm   float64
	// Board is the hardware this node is, by profile name. Empty is a host
	// build. A name nothing matches is refused rather than ignored: the board
	// decides the transmit ceiling, the noise figure and the battery, so a
	// silent fallback would be a different node answering the question.
	Board Board
}

// Place puts one node down and hands back a handle to it.
//
// It inherits its neighbours' regions and their firmware, because somebody
// dropping a repeater on a map is adding a repeater to this network, not
// choosing a firmware strategy.
func (n Nodes) Place(ctx context.Context, p Placement) (Node, error) {
	if p.Kind == "" {
		p.Kind = SimpleRepeater
	}
	params := map[string]any{
		"name": p.Name, "kind": string(p.Kind), "lat": p.Lat, "lon": p.Lon,
	}
	if p.HeightM != 0 {
		params["height_m"] = p.HeightM
	}
	if p.TxDBm != 0 {
		params["tx_dbm"] = p.TxDBm
	}
	if p.Board != "" {
		params["board"] = string(p.Board)
	}
	if err := n.w.Do(ctx, "nodes.place", params); err != nil {
		return Node{}, err
	}
	return Node{w: n.w, name: p.Name}, nil
}

// PlaceMany puts several down, then measures the links once.
//
// One warm at the end rather than one per node: nodes.place re-measures the
// matrix each time, and on a national network that is minutes repeated.
func (n Nodes) PlaceMany(ctx context.Context, ps []Placement) ([]Node, error) {
	out := make([]Node, 0, len(ps))
	for _, p := range ps {
		h, err := n.Place(ctx, p)
		if err != nil {
			return out, err
		}
		out = append(out, h)
	}
	return out, n.w.Do(ctx, "links.recompute", nil)
}

// Delete removes them, in one rebuild.
//
// All or none: a name that is not there refuses and removes nothing, because
// half a deletion leaves a scenario nobody described and no way to tell which
// half survived without asking again.
func (n Nodes) Delete(ctx context.Context, names ...string) error {
	if len(names) == 0 {
		return nil
	}
	return n.w.Do(ctx, "nodes.delete_many", names)
}

// Keep deletes everything these do not name.
//
// The complement is worked out at the workbench rather than here, so it cannot
// be computed against a list that changed in between.
func (n Nodes) Keep(ctx context.Context, names ...string) error {
	return n.w.Do(ctx, "nodes.keep", names)
}

// Select replaces the selection, or adds to it.
func (n Nodes) Select(ctx context.Context, names []string, add bool) error {
	verb := "nodes.select_many"
	if add {
		verb = "nodes.add_to_selection"
	}
	return n.w.Do(ctx, verb, names)
}

// Selected is who is selected now.
func (n Nodes) Selected(ctx context.Context) ([]string, error) {
	all, err := n.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, x := range all {
		if x.Selected {
			out = append(out, x.Name)
		}
	}
	return out, nil
}

// RefreshStats samples what every node is costing, rather than waiting for the
// window to ask.
func (n Nodes) RefreshStats(ctx context.Context) error {
	return n.w.Do(ctx, "nodes.stats", nil)
}

// Node is one node. Live: a handle, not a copy - it holds a name, and asks.
type Node struct {
	w    *Workbench
	name string
}

// Node makes a handle without checking it exists, so a caller can name one
// before placing it. Every method on it will say so if it does not.
func (w *Workbench) Node(name string) Node { return Node{w: w, name: name} }

// Name is what it is called, which is also its identity everywhere.
func (n Node) Name() string { return n.name }

// Info is what the network says about it, now.
func (n Node) Info(ctx context.Context) (NodeInfo, error) {
	return Nodes{n.w}.Get(ctx, n.name)
}

// Start and Stop are its firmware process.
func (n Node) Start(ctx context.Context) error { return n.w.Do(ctx, "node.start", n.name) }
func (n Node) Stop(ctx context.Context) error  { return n.w.Do(ctx, "node.stop", n.name) }

// Output is what this node printed, from one of four voices: "serial" is the
// board's own port (a native node's standard error), "boot" is the ROM's on a
// board whose application talks over USB, "emulator" is what QEMU or Renode
// said about running it, and "radio" is the radio model's log.
//
// The lines, not a count of them. A board that has gone quiet is read by
// looking at what it last said.
func (n Node) Output(ctx context.Context, source string, lines int) ([]string, error) {
	var got struct {
		Tail []string `json:"tail"`
	}
	err := n.w.CallInto(ctx, "node.output", map[string]any{
		"node": n.name, "source": source, "lines": lines}, &got)
	return got.Tail, err
}

// OutputWindow opens one of this node's logs in a window of its own.
//
// A tab is one pane. What people do while a board is misbehaving is watch its
// screen and two of its logs together - what the board printed beside what the
// emulator said about running it - and that needs windows.
func (n Node) OutputWindow(ctx context.Context, source string) error {
	return n.w.Do(ctx, "node.output_window",
		map[string]any{"node": n.name, "source": source})
}

// Card is what is in this node's card slot, and changing it.
//
// A slot is not a fitted card: the board says the slot exists, this says
// whether it is filled. A firmware marked as needing a card fills the slot
// whatever this says, because a build that keeps its settings there boots into
// nothing without one.
func (n Node) Card(ctx context.Context, c CardChange) (CardSlot, error) {
	p := map[string]any{"node": n.name}
	if c.Fitted != nil {
		p["fitted"] = *c.Fitted
	}
	if c.File != nil {
		p["file"] = *c.File
	}
	if c.Wipe {
		p["wipe"] = true
	}
	var out CardSlot
	return out, n.w.CallInto(ctx, "node.card", p, &out)
}

// CardChange is what to change about a node's card. Nil leaves a field alone,
// which is why the two that can be turned off are pointers: "leave this" and
// "take the card out" are different answers and a bool cannot say both.
type CardChange struct {
	Fitted *bool
	// File hands the node a card of its own - shared between runs, or
	// prepared in advance. A pointer to the empty string returns it to its
	// own, named after it and kept beside its flash.
	File *string
	Wipe bool
}

// Wipe puts this board back to factory: its flash, its card, its files.
//
// A board keeps what it was told between runs, as hardware does, so a node
// configured into a corner stays there until this is called. Refused while it
// is running, rather than rewriting a flash underneath the emulator holding it.
func (n Node) Wipe(ctx context.Context) error {
	return n.w.Do(ctx, "node.wipe", map[string]any{"node": n.name})
}

// Delete removes it from the scenario, and re-measures what is left.
func (n Node) Delete(ctx context.Context) error {
	return n.w.Do(ctx, "nodes.delete", map[string]any{"node": n.name})
}

// Move puts it somewhere else. The physics moves with it: cached losses for
// this node are forgotten.
func (n Node) Move(ctx context.Context, lat, lon float64) error {
	return n.w.Do(ctx, "nodes.move", map[string]any{
		"name": n.name, "lat": lat, "lon": lon})
}

// SetRegions is what it relays for.
func (n Node) SetRegions(ctx context.Context, regions ...string) error {
	return n.w.Do(ctx, "nodes.regions", map[string]any{
		"node": n.name, "regions": regions})
}

// SetFirmware changes what it runs.
//
// Applied by default, which means stop, provision, start: firmware is chosen
// when a node launches, so recording it and leaving the node on its old build
// is the control somebody presses twice and then distrusts. Pass false to
// record it for the next start instead - and know that is what you have done.
// Build is the build this node runs, and false when it is pinned to nothing.
//
// The whole row rather than the version string, because deleting a build or
// comparing two needs its path and its board, and reassembling those from a
// version is the kind of guesswork that deletes the wrong file.
func (n Node) Build(ctx context.Context) (Build, bool, error) {
	info, err := n.w.Nodes().Get(ctx, n.name)
	if err != nil || info.Firmware == "" {
		return Build{}, false, err
	}
	all, err := n.w.Firmware().Library(ctx)
	if err != nil {
		return Build{}, false, err
	}
	for _, b := range all {
		if b.Version == info.Firmware {
			return b, true, nil
		}
	}
	return Build{}, false, nil
}

func (n Node) SetFirmware(ctx context.Context, b Build, apply bool) error {
	verb := "node.set_firmware"
	if !apply {
		verb = "node.set_firmware_only"
	}
	return n.w.Do(ctx, verb, map[string]any{
		"node": n.name, "version": b.Version,
		"board": b.Board, "role": b.Role})
}

// SetBoard changes what hardware this node is.
//
// A change to the physics rather than a label, so it rebuilds and re-warms -
// and it clears a firmware pin made for a different board, because that image
// cannot run on this one and a pin nobody can honour reads as a configured
// node right up until it refuses to start.
func (n Node) SetBoard(ctx context.Context, board Board) error {
	return n.w.Do(ctx, "node.set_board",
		map[string]any{"node": n.name, "board": string(board)})
}

// SetTrueRF makes this receiver take waveform verdicts whatever the run's
// mode - the hybrid flag, for measuring one node honestly inside a cheap run.
func (n Node) SetTrueRF(ctx context.Context, on bool) error {
	return n.w.Do(ctx, "node.truerf", map[string]any{"node": n.name, "on": on})
}

// Inject originates a packet without firmware.
//
// It exercises the radio model and the channel; what it does not exercise is
// relaying, which is a firmware behaviour and needs a firmware.
func (n Node) Inject(ctx context.Context) error {
	return n.w.Do(ctx, "sim.inject", n.name)
}

// Provisioning is what this node is told at boot, in the console's own words.
func (n Node) Provisioning(ctx context.Context) ([]string, error) {
	var out struct {
		Commands []string `json:"commands"`
	}
	return out.Commands, n.w.CallInto(ctx, "node.provisioning", n.name, &out)
}

// Serve hands this companion to a real client - meshcore-cli, or an app over a
// bridge - and returns where to point it.
func (n Node) Serve(ctx context.Context, over Transport) (string, error) {
	var out struct {
		Addr string `json:"addr"`
	}
	err := n.w.CallInto(ctx, "bench.serve",
		map[string]any{"node": n.name, "kind": over}, &out)
	return out.Addr, err
}

// Unserve takes it back.
func (n Node) Unserve(ctx context.Context) error {
	return n.w.Do(ctx, "bench.drop", map[string]any{"node": n.name})
}

// Running reports whether its firmware process is up.
func (n Node) Running(ctx context.Context) (bool, error) {
	stats, err := n.w.NodeStats(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range stats {
		if s.Name == n.name {
			return s.Running, nil
		}
	}
	return false, nil
}

// WaitRunning waits for its firmware to come up.
//
// Polling, for now. When the socket learns to push this switches underneath
// and no caller changes - which is why the clients are built before the
// events rather than after.
func (n Node) WaitRunning(ctx context.Context, timeout time.Duration) error {
	return waitFor(ctx, timeout, "firmware on "+n.name, func() (bool, string, error) {
		up, err := n.Running(ctx)
		if err != nil {
			return false, "", err
		}
		if up {
			return true, "", nil
		}
		return false, "not running", nil
	})
}
