package comp

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/theme/brandfont"
)

// The timeline drew red, orange and green marks and said nowhere what they
// meant, so it could only be read by somebody who had read the source.
func TestTheTimelineNamesEveryColourItDraws(t *testing.T) {
	th := theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(brandfont.Collection())))
	kinds := timelineKinds(th)
	if len(kinds) == 0 {
		t.Fatal("the plot draws no kinds at all")
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		if k.label == "" {
			t.Errorf("the %q mark has no word for it", k.kind)
		}
		// Two kinds the same colour is a key that cannot be used: the reader
		// has three marks and two answers.
		key := string([]byte{k.col.R, k.col.G, k.col.B})
		if seen[key] {
			t.Errorf("%q is drawn in a colour another kind already uses", k.kind)
		}
		seen[key] = true
	}
	for _, want := range []string{"tx", "rx", "miss"} {
		found := false
		for _, k := range kinds {
			found = found || k.kind == want
		}
		if !found {
			t.Errorf("events of kind %q are drawn and never explained", want)
		}
	}
}

// The key takes room off the top of the plot, and the plot still fills the
// panel it was given.
func TestTheTimelineKeepsItsSizeWithTheKeyAboveIt(t *testing.T) {
	th := theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(brandfont.Collection())))
	var ops op.Ops
	sz := image.Pt(900, 400)
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz)}
	tl := &Timeline{}
	snap := &state.Snapshot{Events: []state.Event{
		{From: "Abernethy Repeater", Kind: "tx", AtMs: 1000},
		{From: "Abernethy Repeater", To: "Bishop Hill", Kind: "rx", AtMs: 1040},
		{From: "Abernethy Repeater", To: "West Lomond", Kind: "miss", AtMs: 1040},
	}}
	if h := tl.legend(th, gtx, sz.X); h <= 0 {
		t.Errorf("the key is %d px tall, so nothing on screen says what a mark is", h)
	}
	if d := tl.Layout(th, gtx, snap); d.Size != sz {
		t.Errorf("the panel drew %v of the %v it was given", d.Size, sz)
	}
}
