// What the reception ledger says about one message, and which region it was
// scoped to.
//
// Split from packetview.go on length. The seam is real: that file turns one
// frame into a view, and this one answers the two questions that span a whole
// journey - what happened at each node across every hop of it, and who the
// message was addressed to.
package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/packet"
	"github.com/MeshBench/meshbench/internal/sim/capture"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

// scopeOf confirms which region a packet was sent to, if any.
//
// Confirmation, never decoding: the region key is not in the packet, so all
// this can do is compute each candidate's code over the same payload and see
// which one reproduces the code on the wire (provider.NamedRegions, which has
// been written and tested for exactly this and until now was wired to
// nothing). A packet whose scope matches nothing we hold is reported as such
// rather than as unscoped - the two look identical on the wire and mean
// entirely different things.
func (s *Sim) scopeOf(frame []byte, d packet.Dissection) state.PacketScope {
	if !d.HasTransport || len(d.TransportCodes) < 2 {
		return state.PacketScope{}
	}
	sc := state.PacketScope{
		Scoped: true,
		Code:   fmt.Sprintf("%04X", d.TransportCodes[0]),
	}
	if d.TransportCodes[0] == 0 && d.TransportCodes[1] == 0 {
		sc.Note = "addressed to no region"
		return sc
	}
	names := s.regionCandidates()
	sc.Candidates = len(names)
	if len(names) == 0 {
		return sc
	}
	if m := provider.NewNamedRegions(names).Match(frame, d.TransportCodes); len(m) > 0 {
		sc.Name = strings.Join(m, ", ")
	}
	return sc
}

// regionCandidates is every region name the run knows of.
//
// The candidates are the whole limit on what scope matching can identify, so
// they come from the scenario itself - the regions its nodes were configured
// or inferred into - rather than from a hardcoded list that would quietly
// stop recognising a mesh the moment it grew a new one.
func (s *Sim) regionCandidates() []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range s.nodes {
		for _, r := range n.Regions {
			if r != "" && !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	sort.Strings(out)
	return out
}

// collapseLedger reduces the whole journey's reception history to one row
// per node: whichever attempt answers "did this node ever get the message,
// and if not, why not" - not a list of every attempt, which is what turned
// the reception ledger into a wall of near-duplicate rows for a node offered
// the same flood on every hop, and which is also why the table could show
// "never saw it" for a node the "why?" modal - reading the same history -
// could show as accepted: the row that survived to be drawn was whichever
// attempt happened to come first, not the one that mattered most.
//
// Preference order, first match wins, first occurrence (rows arrive already
// in the journey's own chronological order) within that match:
//  1. accepted, or decoded and dropped by the firmware (deliberately, e.g.
//     dedup) - either way the radio genuinely received it, which is the
//     fact this row exists to answer.
//  2. offered and failed with a reason the engine can give.
//  3. never offered at all, on any hop.
//
// full's own order is preserved for the modal, which still wants every
// attempt; this only decides what the one summary row says.
func collapseLedger(full []state.PacketReception) []state.PacketReception {
	type best struct {
		heard, miss, never          state.PacketReception
		hasHeard, hasMiss, hasNever bool
	}
	byNode := map[string]*best{}
	var order []string
	for _, r := range full {
		b, ok := byNode[r.Node]
		if !ok {
			b = &best{}
			byNode[r.Node] = b
			order = append(order, r.Node)
		}
		switch {
		case r.Demod && r.CRCOK:
			if !b.hasHeard {
				b.heard, b.hasHeard = r, true
			}
		case r.Why != "":
			if !b.hasMiss {
				b.miss, b.hasMiss = r, true
			}
		default:
			if !b.hasNever {
				b.never, b.hasNever = r, true
			}
		}
	}
	out := make([]state.PacketReception, 0, len(order))
	for _, n := range order {
		b := byNode[n]
		switch {
		case b.hasHeard:
			out = append(out, b.heard)
		case b.hasMiss:
			out = append(out, b.miss)
		default:
			out = append(out, b.never)
		}
	}
	return out
}

// ledgerAcrossJourney merges the radio ledger's view of every transmission a
// message made into one reception per node.
//
// ForPacket already returns one row per node in the whole scenario for a
// single transmission - that is the ledger's own design, so a node clean out
// of range still gets a row saying so. Doing that once per hop and appending
// would repeat that same full census once per transmission: a two-hop
// journey across 360 nodes would offer 732 rows for 360 receivers, nearly
// all of them the same "never saw it" fact said twice. So every row where
// something measurable happened is kept - a node can legitimately be
// offered on more than one hop, and that is real information - but a node
// offered on no hop at all gets exactly one row for the whole journey, not
// one per transmission it missed entirely.
func ledgerAcrossJourney(ledger capture.Ledger, hops []state.PacketHop) []state.PacketReception {
	// The engine's own words for a failed reception live on the hop that
	// failed it (MissedBy/MissWhy, parallel arrays), not on the ledger row -
	// keyed by transmission and receiver so a row can find its own reason
	// rather than a stranger's.
	type failure struct {
		packet uint64
		node   string
	}
	why := map[failure]string{}
	hopOf := map[uint64]int{}
	for _, h := range hops {
		hopOf[h.PacketID] = h.Hops
		for i, n := range h.MissedBy {
			if i < len(h.MissWhy) {
				why[failure{h.PacketID, n}] = h.MissWhy[i]
			}
		}
	}

	var out []state.PacketReception
	seen := map[string]bool{}
	addRow := func(r capture.Reception) {
		fw := "never saw it"
		switch {
		case r.Demod && r.CRCOK && r.FirmwareSaw:
			fw = "accepted"
		case r.Demod && r.CRCOK:
			fw = "dropped"
		}
		out = append(out, state.PacketReception{
			Node: r.ToNode, From: r.FromNode, Offered: r.Offered,
			RSSIdBm: r.RSSIdBm, SNRdB: r.SNRdB,
			Demod: r.Demod, CRCOK: r.CRCOK, Firmware: fw,
			Why: why[failure{r.PacketID, r.ToNode}],
			Hop: hopOf[r.PacketID],
		})
	}
	for _, h := range hops {
		for _, r := range ledger.ForPacket(h.PacketID) {
			if r.Offered {
				addRow(r)
				seen[r.ToNode] = true
			}
		}
	}
	for _, h := range hops {
		for _, r := range ledger.ForPacket(h.PacketID) {
			if !r.Offered && !seen[r.ToNode] {
				seen[r.ToNode] = true
				addRow(r)
			}
		}
	}
	return out
}
