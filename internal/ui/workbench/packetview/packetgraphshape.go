// Turning a journey's raw receptions into a set a person can actually read:
// which edge speaks for each receiver, how deep each node really sits, which
// nodes a crowded layer can afford to drop, and what order keeps the lines
// from crossing. All four exist because the honest raw data - every node's
// every attempt at every hop - is far more than a picture can carry.
package packetview

import "sort"

// drawableSet decides which nodes survive maxRowsPerLayer, and then removes
// whatever that choice invalidated.
//
// Two rules, both learned from the same bug - a column of nodes with nothing
// arriving at them:
//
// Which nodes a full layer keeps decides whether the *next* column still
// parses. Dropping a node that relayed onward strands everything it fed, so a
// layer keeps its relays before its leaves; a leaf's absence costs the picture
// one dot, a relay's costs it a whole subtree's worth of explanation.
//
// And whatever still falls off takes its edges with it. drawHopGraph skips an
// edge whose endpoint it cannot place, which is the right thing for it to do
// and silent - so the pruning has to happen here, where it can also be
// counted. Repeated until nothing more changes, because pruning an orphan can
// orphan its own children in turn.
//
// names must already be ordered by (layer, pos); origin is never dropped.
func drawableSet(names []string, layer map[string]int, pos map[string]float64,
	edges []hopEdge, origin string) (kept map[string]bool, surviving []hopEdge, hidden int) {
	relays := map[string]bool{}
	for _, e := range edges {
		relays[e.From] = true
	}
	byLayer := map[int][]string{}
	for _, n := range names {
		byLayer[layer[n]] = append(byLayer[layer[n]], n)
	}
	kept = map[string]bool{}
	for _, ns := range byLayer {
		if len(ns) > maxRowsPerLayer {
			ranked := append([]string(nil), ns...)
			sort.SliceStable(ranked, func(i, j int) bool {
				if relays[ranked[i]] != relays[ranked[j]] {
					return relays[ranked[i]]
				}
				return pos[ranked[i]] < pos[ranked[j]]
			})
			hidden += len(ranked) - maxRowsPerLayer
			ns = ranked[:maxRowsPerLayer]
		}
		for _, n := range ns {
			kept[n] = true
		}
	}
	if origin != "" {
		kept[origin] = true
	}

	for {
		surviving = surviving[:0]
		touched := map[string]bool{}
		for _, e := range edges {
			if kept[e.From] && kept[e.To] {
				surviving = append(surviving, e)
				touched[e.From], touched[e.To] = true, true
			}
		}
		orphaned := false
		for n := range kept {
			if n == origin || touched[n] {
				continue
			}
			delete(kept, n)
			hidden++
			orphaned = true
		}
		if !orphaned {
			return kept, surviving, hidden
		}
	}
}

// collapseEdgesPerReceiver keeps, for each node that received anything, only
// the one edge that answers "did this node ever get the message": its first
// successful reception (OK, in hopEdge's terms) if it ever had one, or else
// its first reason for missing it. edges is walked in the order it was
// built, which is already the journey's own chronological order, so "first"
// here really does mean first.
func collapseEdgesPerReceiver(edges []hopEdge) []hopEdge {
	type slot struct {
		heard, miss       hopEdge
		hasHeard, hasMiss bool
	}
	byReceiver := map[string]*slot{}
	var order []string
	for _, e := range edges {
		s, ok := byReceiver[e.To]
		if !ok {
			s = &slot{}
			byReceiver[e.To] = s
			order = append(order, e.To)
		}
		switch {
		case e.OK() && !s.hasHeard:
			s.heard, s.hasHeard = e, true
		case !e.OK() && !s.hasMiss:
			s.miss, s.hasMiss = e, true
		}
	}
	out := make([]hopEdge, 0, len(order))
	for _, n := range order {
		s := byReceiver[n]
		if s.hasHeard {
			out = append(out, s.heard)
		} else {
			out = append(out, s.miss)
		}
	}
	return out
}

// refineLayersFromOrigin overwrites layer, for every node the walk reaches,
// with its true distance along edges - breadth-first outward from origin,
// one hop per edge crossed. Bounded by a visited set, so a loop in which
// relay heard which (the mesh's own topology can have one even though the
// message itself only ever moves forward) cannot walk this forever.
func refineLayersFromOrigin(origin string, edges []hopEdge, layer map[string]int) {
	if origin == "" {
		return
	}
	children := map[string][]hopEdge{}
	for _, e := range edges {
		children[e.From] = append(children[e.From], e)
	}
	layer[origin] = 0
	visited := map[string]bool{origin: true}
	queue := []string{origin}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, e := range children[n] {
			if visited[e.To] {
				continue
			}
			visited[e.To] = true
			layer[e.To] = layer[n] + 1
			queue = append(queue, e.To)
		}
	}
}

// barycenterOrder refines a per-layer seed ordering by repeatedly moving
// each node toward the mean position of what it connects to, several sweeps
// in each direction - the standard fix for the crossing a naive "first
// seen" layer order produces. names is every node the caller wants
// positioned; layer and seed must have an entry for each of them.
//
// The forward sweep (shallow to deep) uses only edges that succeeded: a
// node's row should track who actually relayed to it, not everyone who
// merely tried and failed. The backward sweep uses every edge, so a layer
// also settles toward what is downstream of it - without it, the last
// layer in a run never gets a say in anything upstream of it.
func barycenterOrder(names []string, layer, seed map[string]int, edges []hopEdge) map[string]float64 {
	pos := make(map[string]float64, len(names))
	byLayer := map[int][]string{}
	maxLayer := 0
	for _, n := range names {
		pos[n] = float64(seed[n])
		l := layer[n]
		byLayer[l] = append(byLayer[l], n)
		if l > maxLayer {
			maxLayer = l
		}
	}
	for _, ns := range byLayer {
		sort.SliceStable(ns, func(i, j int) bool { return pos[ns[i]] < pos[ns[j]] })
	}

	heardFrom := func(n string) []string {
		var out []string
		for _, e := range edges {
			if e.To == n && e.Why == missNone {
				out = append(out, e.From)
			}
		}
		return out
	}
	anyNeighbour := func(n string) []string {
		var out []string
		for _, e := range edges {
			switch n {
			case e.From:
				out = append(out, e.To)
			case e.To:
				out = append(out, e.From)
			}
		}
		return out
	}
	resettle := func(ns []string, neighboursOf func(string) []string) {
		at := make([]float64, len(ns))
		for i, n := range ns {
			sum, count := 0.0, 0
			for _, nb := range neighboursOf(n) {
				if v, ok := pos[nb]; ok {
					sum += v
					count++
				}
			}
			at[i] = pos[n]
			if count > 0 {
				at[i] = sum / float64(count)
			}
		}
		for i, n := range ns {
			pos[n] = at[i]
		}
		sort.SliceStable(ns, func(i, j int) bool { return pos[ns[i]] < pos[ns[j]] })
		for i, n := range ns {
			pos[n] = float64(i)
		}
	}

	const sweeps = 4
	for s := 0; s < sweeps; s++ {
		for l := 1; l <= maxLayer; l++ {
			resettle(byLayer[l], heardFrom)
		}
		for l := maxLayer - 1; l >= 0; l-- {
			resettle(byLayer[l], anyNeighbour)
		}
	}
	return pos
}
