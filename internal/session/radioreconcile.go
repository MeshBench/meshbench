// What the node says it is, against what the model assumed.
//
// The engine computes every path loss from the scenario's transmit power and
// modulation. The node has its own, set by its firmware, by provisioning, or
// by somebody typing at it - and until now nothing compared the two, so they
// could diverge silently and the difference would land in whatever term was
// being fitted to cover the gap.
//
// A companion reports all of it in the SelfInfo frame it already sends, so
// this costs one query and a comparison. Asking is better than assuming even
// when the assumption is usually right, because the case that matters is
// exactly the one where it is not.
package session

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func registerRadioReconcile(st *state.Store, s *Sim) {
	// node.radio: what the scenario assumes, what the node reports, and
	// whether they agree.
	st.Handle("node.radio", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			name = selectedName(w)
		}
		if name == "" {
			return nil, fmt.Errorf("node.radio needs a node")
		}
		var scen *scenarioRadio
		for _, n := range s.nodes {
			if n.Name == name {
				r := radioOf(n)
				scen = &r
				break
			}
		}
		if scen == nil {
			return nil, fmt.Errorf("no node named %q", name)
		}
		out := map[string]any{"node": name, "assumed": scen.describe()}

		// A repeater has no companion protocol, and both backends share the
		// same bridge, so its own CLI is the way to ask it. This is why the
		// split that matters is companion against repeater rather than native
		// against emulated: the seam is the bridge, and every backend has one.
		if !isCompanionNode(s.nodes, name) {
			rep, note, err := s.askRepeaterRadio(name)
			if err != nil {
				return nil, err
			}
			if rep == nil {
				out["reported"], out["note"] = nil, note
				return out, nil
			}
			out["reported"] = rep.describe()
			diff := scen.differences(*rep)
			out["differences"] = diff
			if len(diff) > 0 {
				w.Say(fmt.Sprintf("%s disagrees with the model: %v", name, diff))
			} else {
				w.Say(name + ": the node agrees with the model")
			}
			return out, nil
		}

		c, ok := s.comps[name]
		if !ok {
			// Ask, then come back: a companion answers this in a frame, and
			// the frame arrives when the engine next steps.
			if err := s.connectCompanion(name); err != nil {
				out["reported"] = nil
				out["note"] = "not a connected companion: " + err.Error()
				return out, nil
			}
			c = s.comps[name]
		}
		if err := s.compFrame(name, proto.DeviceQuery()); err != nil {
			return nil, err
		}
		c.mu.Lock()
		self := c.self
		c.mu.Unlock()
		if self == nil {
			out["reported"] = nil
			out["note"] = "asked; the answer arrives when the engine next steps, " +
				"so call this again once it has"
			return out, nil
		}

		rep := scenarioRadio{
			FreqMHz: float64(self.FreqKHz) / 1000,
			BwHz:    float64(self.BWKHz) * 1000,
			SF:      int(self.SF), CR: int(self.CR),
			TxDBm: float64(self.TxPowerDBm),
		}
		out["reported"] = rep.describe()
		diff := scen.differences(rep)
		out["differences"] = diff
		if len(diff) == 0 {
			w.Say(name + ": the node agrees with the model")
		} else {
			// Loudly. A model computing path loss from a transmit power the
			// node does not have is wrong by exactly that difference, and
			// nothing else in the run will say so.
			w.Say(fmt.Sprintf("%s disagrees with the model: %v", name, diff))
		}
		return out, nil
	})

	// node.radio_adopt: believe the node.
	//
	// The node is the authority on what it is set to. The scenario is a
	// statement of intent, and where they differ it is the scenario that is
	// out of date.
	st.Handle("node.radio_adopt", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			name = selectedName(w)
		}
		c, ok := s.comps[name]
		if !ok || c == nil {
			return nil, fmt.Errorf("%s is not a connected companion", name)
		}
		c.mu.Lock()
		self := c.self
		c.mu.Unlock()
		if self == nil {
			return nil, fmt.Errorf("%s has not reported yet; ask with node.radio first", name)
		}
		changed := false
		for i := range s.nodes {
			if s.nodes[i].Name != name {
				continue
			}
			s.nodes[i].TxPowerDBm = float64(self.TxPowerDBm)
			s.nodes[i].Radio.CentreHz = float64(self.FreqKHz) * 1000
			s.nodes[i].Radio.BandwidthHz = float64(self.BWKHz) * 1000
			s.nodes[i].Radio.SpreadFactor = int(self.SF)
			s.nodes[i].Radio.CodingRate = int(self.CR)
			changed = true
		}
		if !changed {
			return nil, fmt.Errorf("no node named %q", name)
		}
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				w.Nodes[i].TxDBm = float64(self.TxPowerDBm)
			}
		}
		// The engine caches path loss per pair, so a changed transmit power
		// has to invalidate it or every link keeps the old answer.
		s.buildSeeded(s.nodes, s.freqMHz, s.seed)
		w.Links = nil
		s.warm(st, len(s.nodes))
		w.Say(fmt.Sprintf("%s: model now uses the node's own %d dBm", name, self.TxPowerDBm))
		return map[string]any{"node": name, "tx_dbm": self.TxPowerDBm}, nil
	})
}

// scenarioRadio is the modulation and power one node is modelled with.
type scenarioRadio struct {
	FreqMHz float64
	BwHz    float64
	SF, CR  int
	TxDBm   float64
}

func (r scenarioRadio) describe() map[string]any {
	return map[string]any{
		"freq_mhz": r.FreqMHz, "bandwidth_hz": r.BwHz,
		"spreading_factor": r.SF, "coding_rate": r.CR, "tx_dbm": r.TxDBm,
		"coding_rate_note": "model and node use different conventions for this; " +
			"MeshCore's CLI takes the denominator, 5 to 8",
	}
}

// differences lists only what disagrees, so an empty list means agreement
// rather than "nothing was checked".
func (r scenarioRadio) differences(o scenarioRadio) []string {
	var out []string
	if math.Abs(r.TxDBm-o.TxDBm) > 0.5 {
		out = append(out, fmt.Sprintf("transmit power: model %.0f dBm, node %.0f dBm",
			r.TxDBm, o.TxDBm))
	}
	if o.FreqMHz > 0 && math.Abs(r.FreqMHz-o.FreqMHz) > 0.001 {
		out = append(out, fmt.Sprintf("frequency: model %.3f MHz, node %.3f MHz",
			r.FreqMHz, o.FreqMHz))
	}
	if o.BwHz > 0 && math.Abs(r.BwHz-o.BwHz) > 1 {
		out = append(out, fmt.Sprintf("bandwidth: model %.0f Hz, node %.0f Hz",
			r.BwHz, o.BwHz))
	}
	if o.SF > 0 && r.SF != o.SF {
		out = append(out, fmt.Sprintf("spreading factor: model %d, node %d", r.SF, o.SF))
	}
	// Coding rate is not compared, and saying so is better than comparing it
	// wrongly.
	//
	// MeshCore's CLI takes the denominator, 5 to 8, so 4/5 is "5". The
	// scenario stores the numerator's partner in its own convention, and the
	// two are the same setting written differently. Flagging that as a
	// disagreement would be a false alarm on every node, which would teach
	// somebody to ignore this check - and the whole value of it is that a
	// difference means something.
	_ = o.CR
	return out
}

func selectedName(w *state.World) string {
	for i := range w.Nodes {
		if w.Nodes[i].Selected {
			return w.Nodes[i].Name
		}
	}
	return ""
}

// radioOf is what the scenario says one node transmits with.
//
// Falling back to the engine's own defaults where the node carries none, so
// the comparison is against what is actually being modelled rather than
// against a zero the model never used.
func radioOf(n scenario.Node) scenarioRadio {
	r := scenarioRadio{TxDBm: n.TxPowerDBm}
	r.FreqMHz = n.Radio.CentreHz / 1e6
	if r.FreqMHz <= 0 {
		r.FreqMHz = 869.618
	}
	r.BwHz = n.Radio.BandwidthHz
	if r.BwHz <= 0 {
		r.BwHz = 250e3
	}
	r.SF = n.Radio.SpreadFactor
	if r.SF <= 0 {
		r.SF = 10
	}
	r.CR = n.Radio.CodingRate
	if r.CR <= 0 {
		r.CR = 1
	}
	return r
}

// isCompanionNode reports whether this node speaks the companion protocol.
func isCompanionNode(nodes []scenario.Node, name string) bool {
	for _, n := range nodes {
		if n.Name == name {
			return n.Kind == scenario.Companion
		}
	}
	return false
}

// askRepeaterRadio asks a repeater over its own console.
//
// MeshCore's CLI answers "get radio" with "> freq,bw,sf,cr" and "get tx" with
// the power. Both native and emulated repeaters answer it, because both are
// the same firmware behind the same bridge - the emulator changes what runs
// the instructions, not what the instructions say.
func (s *Sim) askRepeaterRadio(name string) (*scenarioRadio, string, error) {
	buf, err := s.consoleFor(name)
	if err != nil {
		return nil, "", err
	}
	en, ok := s.eng.NodeByName(name)
	if !ok || en.Firmware == nil {
		return nil, "", fmt.Errorf("%s runs no firmware", name)
	}
	mark := buf.Mark()
	if err := en.Firmware.Bridge.Type([]byte("get radio\r\n")); err != nil {
		return nil, "", err
	}
	if err := en.Firmware.Bridge.Type([]byte("get tx\r\n")); err != nil {
		return nil, "", err
	}
	// The reply lands when the engine next steps, so this reads what has
	// arrived rather than waiting on the goroutine that does the stepping.
	lines := buf.LinesSince(mark)
	r := scenarioRadio{}
	seen := false
	for _, l := range lines {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), ">"))
		if strings.Count(l, ",") == 3 {
			var freq, bw float64
			var sf, cr int
			if n, _ := fmt.Sscanf(l, "%f,%f,%d,%d", &freq, &bw, &sf, &cr); n == 4 {
				r.FreqMHz, r.BwHz, r.SF, r.CR = freq, bw*1000, sf, cr
				seen = true
			}
			continue
		}
		if v, err := strconv.ParseFloat(l, 64); err == nil && v > 0 && v <= 40 {
			r.TxDBm, seen = v, true
		}
	}
	if !seen {
		return nil, "asked over its console; the reply lands when the engine " +
			"next steps, so call this again once it has", nil
	}
	return &r, "", nil
}
