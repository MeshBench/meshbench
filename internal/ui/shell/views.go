// Views: what each one is for, and the panels it starts with.
//
// A view is a shape chosen for a kind of work - a starting point, not a cage.
// The declared arrangement is the preset; what is on screen is the live
// arrangement in layoutstate.go, which docking edits and "reset view layout"
// puts back. The fixed-arrangement rule went because it had one consequence
// nobody wanted: twenty of the thirty-three panels could only be reached by
// leaving the window, and there was no way back in.
package shell

// View is one kind of work.
type View int

const (
	Plan View = iota
	Run
	Debug
	// Validate was called Verify, which named no work anybody does. What it
	// holds is the comparison against a real network, which is what its
	// panels and its verbs were already called.
	Validate
	Bench
	App
	numViews
)

func (v View) String() string {
	switch v {
	case Run:
		return "Run"
	case Debug:
		return "Debug"
	case Validate:
		return "Validate"
	case Bench:
		return "Bench"
	case App:
		return "App"
	default:
		return "Plan"
	}
}

// Purpose is what the view is for, in the operator's terms. A six-tab strip
// should not need a manual.
func (v View) Purpose() string {
	switch v {
	case Run:
		return "exercise it and watch: play, schedule traffic, consoles, live feed"
	case Debug:
		return "ask why one thing happened: packets, waterfall, consoles, budgets"
	case Validate:
		return "measure it against the real network: fetch what was heard, compare, correct for the difference"
	case Bench:
		return "compare configurations: sweep a parameter, repeat it, read what differed"
	case App:
		return "write a client against it: an endpoint, the protocol decoded, faults on a button"
	default:
		return "build and site: import, place, drag, boundary, coverage"
	}
}

// Arrangement is how a view divides the window.
//
// This replaced one main panel and a rail of side panels. That shape is right
// for a map with two readouts beside it and wrong for anything that compares:
// the Bench got a run list filling the window and its controls squeezed into a
// 340dp strip, because a strip was the only other place available. A view that
// reads a table against its controls needs columns, and nothing here could say
// so.
//
// Its methods take both kinds of receiver, deliberately. LoadLayouts clones
// straight out of a map and a map element is not addressable, so clone() must
// take a value or the one call site that matters will not compile. The
// mutating method takes a pointer and is only ever called on the addressable
// copy clone() returns, which is the ordering that makes the mix safe.
//
//nolint:recvcheck // clone() is called on a map element and must take a value
type Arrangement struct {
	// Rail is a fixed-width column down the right, drawn full height. Empty
	// for a view that wants none.
	Rail   []Col
	RailDp int

	// Rows divide everything left of the rail, top to bottom.
	Rows []Row
}

// Row is a horizontal band of one or more regions side by side.
type Row struct {
	// Weight is this row's share of the height, against the other rows.
	Weight int
	Cols   []Col
}

// Col is one region of the window, holding one or more panels as tabs.
// WidthDp fixes it; zero takes what the fixed columns leave.
//
// Fixed is for a column of controls, whose fields have a width below which
// they stop being usable. Flexed is for the thing being read.
//
// Tabs rather than a single name because a region is somewhere a panel can be
// put: docking one adds a tab here rather than sending the panel out of the
// window, which is what "show me the waterfall" used to do.
type Col struct {
	Tabs []string
	// Active is which tab is drawn, an index into Tabs.
	Active  int
	WidthDp int
}

// col is one region holding one panel, which is what every preset declares.
func col(name string, widthDp int) Col {
	return Col{Tabs: []string{name}, WidthDp: widthDp}
}

// shown is the panel a region is currently drawing, or "" when every tab in
// it has been closed.
func (c Col) shown() string {
	if c.Active < 0 || c.Active >= len(c.Tabs) {
		return ""
	}
	return c.Tabs[c.Active]
}

// presetFor is the shape each view starts in, and the shape "reset view
// layout" puts it back to. Changing a view's starting shape is changing this
// table; changing what is on screen right now is docking.
func presetFor(v View) Arrangement {
	switch v {
	case Run:
		return withRail("Map", 340, "Schedule", "Scoreboard")
	case Debug:
		return withRail("Packet timeline", 340, "Inspector", "Link")
	case Validate:
		return withRail("Compare", 340, "Validate", "Scoreboard")
	case Bench:
		// Three columns and a strip, because comparing arms means reading a
		// table against the controls that produced it while the runs tick over
		// beside both. Sweep is fixed at a width its fields stay usable at;
		// Results takes the rest, being the thing actually read; Timelines gets
		// the full width under them because a burst is long and thin.
		return Arrangement{
			Rail:   []Col{col("Runs", 0), col("Experiment log", 0), col("Matrix", 0)},
			RailDp: 460,
			Rows: []Row{
				{Weight: 3, Cols: []Col{col("Sweep", 440), col("Results", 0)}},
				{Weight: 2, Cols: []Col{col("Timelines", 0)}},
			},
		}
	case App:
		return withRail("Companion bench", 340, "Events", "Console")
	default:
		return withRail("Map", 340, "Nodes", "Inspector")
	}
}

// withRail is the older shape, kept because most views want it: one panel with
// a fixed rail of readouts beside it.
func withRail(main string, railDp int, rail ...string) Arrangement {
	a := Arrangement{
		RailDp: railDp,
		Rows:   []Row{{Weight: 1, Cols: []Col{col(main, 0)}}},
	}
	for _, n := range rail {
		a.Rail = append(a.Rail, col(n, 0))
	}
	return a
}

// clone is a deep copy, so editing a view's live arrangement cannot reach
// back into the preset every reset reads from.
// Value receiver, deliberately: see the note on the type.
func (a Arrangement) clone() Arrangement {
	out := Arrangement{RailDp: a.RailDp}
	for _, c := range a.Rail {
		out.Rail = append(out.Rail, c.clone())
	}
	for _, r := range a.Rows {
		nr := Row{Weight: r.Weight}
		for _, c := range r.Cols {
			nr.Cols = append(nr.Cols, c.clone())
		}
		out.Rows = append(out.Rows, nr)
	}
	return out
}

func (c Col) clone() Col {
	return Col{Tabs: append([]string(nil), c.Tabs...), Active: c.Active, WidthDp: c.WidthDp}
}

// NumViews is how many views there are, for anything outside this package
// that has to enumerate them - a verb naming one, or a test covering all.
const NumViews = numViews

// PanelsIn is every panel a view starts with, for anything outside this
// package that needs to know where a panel belongs by default - a verb
// choosing which view to switch to, or a test covering the presets.
//
// The preset rather than the live arrangement: what a view is *for* does not
// change because somebody docked a waterfall into it.
func PanelsIn(v View) []string {
	return presetFor(v).panels()
}

// panels is every panel an arrangement holds, tabs included.
func (a Arrangement) panels() []string {
	var out []string
	for _, r := range a.Rows {
		for _, c := range r.Cols {
			out = append(out, c.Tabs...)
		}
	}
	for _, c := range a.Rail {
		out = append(out, c.Tabs...)
	}
	return out
}
