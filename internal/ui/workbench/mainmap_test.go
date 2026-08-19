package workbench

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

// Completing a pair must put the answer on screen. The Link panel lives in
// one view's rail; from anywhere else, a completed pick filled a panel
// nobody could see - a click that visibly did nothing.
func TestCompletingALinkOpensTheLinkPanel(t *testing.T) {
	st := state.New(10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	newRig := func() (*comp.MapView, *shell.Shell, *[]string) {
		mv := &comp.MapView{}
		sh := shell.New()
		popped := []string{}
		sh.OnPopOut = func(name string) { popped = append(popped, name) }
		sh.PoppedOut = func(string) bool { return false }
		wireMapTools(mv, &mapTools{mv: mv}, sh, st, ctx)
		return mv, sh, &popped
	}
	pair := func(mv *comp.MapView) {
		mv.OnLinkPair(comp.LinkEnd{Node: "a", Lat: 56, Lon: -3},
			comp.LinkEnd{Node: "b", Lat: 56.1, Lon: -3.1})
	}

	// From a view without the Link panel, completing the pair pops it out.
	mv, sh, popped := newRig()
	sh.View = shell.Plan
	pair(mv)
	if len(*popped) != 1 || (*popped)[0] != "Link" {
		t.Fatalf("completing a pair in the Plan view popped %v, want the Link panel", *popped)
	}

	// In the view that already shows it, nothing opens.
	mv, sh, popped = newRig()
	sh.View = shell.Debug
	pair(mv)
	if len(*popped) != 0 {
		t.Fatalf("the Link panel is already in the Debug view's rail, yet %v was popped", *popped)
	}

	// And a window already open is left alone.
	mv, sh, popped = newRig()
	sh.View = shell.Plan
	sh.PoppedOut = func(name string) bool { return name == "Link" }
	pair(mv)
	if len(*popped) != 0 {
		t.Fatalf("the Link window was already open, yet %v was popped again", *popped)
	}
}
