package ui

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// dupeGeography answers the "where" half of "where and when does loop detection
// help".
//
// The matrix says how much duplication each arm produced; it does not say where
// it landed, and that turns out to be the more useful number. Duplication is not
// spread evenly - it concentrates wherever the mesh is dense enough that several
// repeaters hear each other, and those are the places loop detection is doing
// work. In a thin chain there is nothing to suppress, so the same setting costs
// its false drops and buys nothing.
//
// Summed over every run, because a node that reflects packets does it under
// every arm and the point here is the position, not the parameter.
func dupeGeography(a *App, e *experiment) string {
	if len(e.results) == 0 {
		return ""
	}
	total := map[string]int{}
	grand := 0
	for _, r := range e.results {
		for node, n := range r.DupePerNode {
			total[node] += n
			grand += n
		}
	}
	if grand == 0 {
		return `<h2>Where the duplication landed</h2>` +
			`<p class="sub">No duplicate deliveries in any run. Either loop detection ` +
			`suppressed all of it, or this network has no redundant paths to suppress - ` +
			`the arms with loop detection off tell you which.</p>`
	}

	pos := map[string]string{}
	kind := map[string]string{}
	for i := range a.Nodes {
		n := &a.Nodes[i]
		pos[n.Name] = fmt.Sprintf("%.3f, %.3f", n.Position.Lat, n.Position.Lon)
		kind[n.Name] = string(n.Kind)
	}

	names := make([]string, 0, len(total))
	for n := range total {
		names = append(names, n)
	}
	// Ties broken by name so the table is stable between exports - a report that
	// reorders itself run to run cannot be diffed.
	sort.Slice(names, func(i, j int) bool {
		if total[names[i]] != total[names[j]] {
			return total[names[i]] > total[names[j]]
		}
		return names[i] < names[j]
	})
	if len(names) > dupeTableRows {
		names = names[:dupeTableRows]
	}

	var b strings.Builder
	shown := 0
	for _, n := range names {
		shown += total[n]
	}
	fmt.Fprintf(&b, `<h2>Where the duplication landed</h2>`+
		`<p class="sub">%d redundant deliveries across every run. The %d nodes below took `+
		`%.0f%% of them - duplication concentrates where the mesh is dense, and those are `+
		`the places loop detection has something to suppress.</p>`,
		grand, len(names), float64(shown)/float64(grand)*100)
	b.WriteString(`<div class="scroll"><table><tr><th>node</th><th>kind</th>` +
		`<th>lat, lon</th><th>redundant deliveries</th><th>share</th></tr>`)
	for _, n := range names {
		fmt.Fprintf(&b, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%.1f%%</td></tr>`,
			html.EscapeString(n), html.EscapeString(kind[n]), html.EscapeString(pos[n]),
			total[n], float64(total[n])/float64(grand)*100)
	}
	b.WriteString(`</table></div>`)
	return b.String()
}

// dupeTableRows bounds the table. Every node appears in the per-run data; this
// is the summary, and a two-hundred-row table is not one.
const dupeTableRows = 25
