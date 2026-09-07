package workbench

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// The panel drew p.head twice when there was nothing to compare: as a section
// title running into the window edge, and again centred in the body with
// nothing between them. A title is what the pane is; the sentence in the
// middle is what to do about it.
func TestTheCompareTitleIsNotTheEmptyState(t *testing.T) {
	var p comparePanel

	for _, tc := range []struct {
		what string
		runs []session.RunRecord
	}{
		{"no saved runs", nil},
		{"one saved run", []session.RunRecord{{Name: "before"}}},
	} {
		p.rebuild(tc.runs)
		if len(p.rows) != 0 {
			t.Fatalf("%s: built %d rows", tc.what, len(p.rows))
		}
		title := compareTitle(&p)
		if title == p.head {
			t.Errorf("%s: the title and the empty-state sentence are the same "+
				"string, so the panel says it twice: %q", tc.what, title)
		}
		if !strings.Contains(p.head, "save") {
			t.Errorf("%s: the empty state does not say what to do: %q", tc.what, p.head)
		}
	}

	// With two runs the title is the comparison itself, which is worth saying
	// at the top rather than a constant.
	// Two runs with a metric between them, since a comparison with no shared
	// metric names builds no rows.
	p.rebuild([]session.RunRecord{
		{Name: "after", Metrics: map[string]float64{"delivered": 12}},
		{Name: "before", Metrics: map[string]float64{"delivered": 9}},
	})
	if len(p.rows) == 0 {
		t.Fatal("two runs built no rows")
	}
	if got := compareTitle(&p); got != p.head {
		t.Errorf("with a comparison the title is %q, want the comparison %q", got, p.head)
	}
}
