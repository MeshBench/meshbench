// The network: what is in it, and what one node can be asked to do.
package client

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

// OfKind filters. Evaluated here rather than at the workbench: it is a
// question about a list somebody already has.
func (n Nodes) OfKind(ctx context.Context, kind string) ([]NodeInfo, error) {
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
	Kind     string
	Lat, Lon float64
	// HeightM and TxDBm default to the scenario's own defaults when zero -
	// ten metres and 22 dBm - rather than to nothing.
	HeightM float64
	TxDBm   float64
	// Board is the hardware this node is, by profile name. Empty is a host
	// build. A name nothing matches is refused rather than ignored: the board
	// decides the transmit ceiling, the noise figure and the battery, so a
	// silent fallback would be a different node answering the question.
	Board string
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
		"name": p.Name, "kind": p.Kind, "lat": p.Lat, "lon": p.Lon,
	}
	if p.HeightM != 0 {
		params["height_m"] = p.HeightM
	}
	if p.TxDBm != 0 {
		params["tx_dbm"] = p.TxDBm
	}
	if p.Board != "" {
		params["board"] = p.Board
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
func (n Node) SetBoard(ctx context.Context, board string) error {
	return n.w.Do(ctx, "node.set_board",
		map[string]any{"node": n.name, "board": board})
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
func (n Node) Serve(ctx context.Context, kind string) (string, error) {
	var out struct {
		Addr string `json:"addr"`
	}
	err := n.w.CallInto(ctx, "bench.serve",
		map[string]any{"node": n.name, "kind": kind}, &out)
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
