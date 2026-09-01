package comp

import (
	"image"
	"sync"
	"testing"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/theme/brandfont"
)

// Two windows drawing their own menus at the same time.
//
// Every pop-out window runs its own frame loop on its own goroutine, and the
// actions a node offers are the same strings for every node. Row state shared
// between two of those loops is a concurrent map write, which Go does not let
// the process survive: no recovery, no useful stack, and the run goes with it.
// Under -race this fails long before it gets that far.
func TestMenuRowsInTwoWindowsDoNotShareState(t *testing.T) {
	items := []MenuItem{
		{Label: "Stop this node", Action: "node.stop"},
		{Label: "Change its firmware", Action: "ui.firmware"},
		{Label: "Coverage from here", Action: "coverage.compute"},
	}
	rows := []*MenuRow{{}, {}}
	var wg sync.WaitGroup
	for i, row := range rows {
		wg.Add(1)
		go func(row *MenuRow, node string) {
			defer wg.Done()
			// A theme and an op buffer of its own, as a real window has: Gio's
			// shaper is not safe for concurrent use either, and sharing one
			// here would prove nothing about this map.
			th := theme.New(theme.Dark, theme.Default,
				text.NewShaper(text.WithCollection(brandfont.Collection())))
			var ops op.Ops
			var r input.Router
			for range 200 {
				ops.Reset()
				gtx := layout.Context{
					Ops:         &ops,
					Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
					Constraints: layout.Exact(image.Pt(600, 80)),
					Source:      r.Source(),
				}
				row.Layout(th, gtx, items, node)
				r.Frame(&ops)
			}
		}(row, [2]string{"Abernethy Repeater", "Bishop Hill"}[i])
	}
	wg.Wait()
}
