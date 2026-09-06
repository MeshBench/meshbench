package workbench

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

// drawCards lays the summary out at one pane size and reports its height.
func drawCards(t *testing.T, w, h int) int {
	t.Helper()
	th := theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(brandfont.Collection())))
	p := &eventsPanel{}
	snap := &state.Snapshot{EventTotal: 22, NowMs: 21100}
	snap.Counts = state.EventCounts{Sent: 8, Received: 14}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(w, h)},
	}
	return p.cards(th, gtx, snap).Size.Y
}

// In a pane too short for both, the class cards give way to the chips.
//
// They are the only numbers on this page said twice - every count on them is
// on the chip that filters it - so what goes is the percentage rather than the
// figure. Keeping all ten put three of the eight filters under the panel below,
// where they could not be pressed, while the counts they filter sat in full
// view above them.
func TestTheClassCardsGiveWayInAShortPane(t *testing.T) {
	const rail = 340 // about what a docked rail offers

	tall := drawCards(t, rail, 1000)
	short := drawCards(t, rail, 430) // about what the App view's rail gives

	if short >= tall {
		t.Errorf("the summary did not shrink for a short pane: %d px at 430, "+
			"%d px at 1000 - so the chips still have nowhere to go", short, tall)
	}
}

// Total events and Duration stay whatever happens: nothing else on the page
// says either, so they are not the cards' to give.
func TestTheSummaryNeverEmpties(t *testing.T) {
	for _, h := range []int{200, 430, 599, 600, 1000} {
		if got := drawCards(t, 340, h); got <= 0 {
			t.Errorf("at a pane %d px tall the summary drew nothing", h)
		}
	}
}

// A pane with the room keeps every card, so nothing changes where nothing was
// wrong: a panel filling the window looks as it did.
func TestATallPaneKeepsEveryCard(t *testing.T) {
	wide := drawCards(t, 1200, 1000)
	rail := drawCards(t, 340, 1000)
	if wide <= 0 || rail <= 0 {
		t.Fatal("the summary drew nothing at a size that has room for it")
	}
	// Ten cards over a narrow pane wrap onto more rows than over a wide one,
	// which is the sign they are all still there.
	if rail <= wide {
		t.Errorf("ten cards in 340 px should wrap taller than in 1200 px; "+
			"got %d and %d", rail, wide)
	}
}
