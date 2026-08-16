// Views: what each one is for, and the panels it declares.
//
// A view is a fixed arrangement chosen for a kind of work, which is the
// deliberate departure from a dock model where the arrangement drifts.
package shell

// View is one kind of work.
type View int

const (
	Plan View = iota
	Run
	Debug
	Verify
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
	case Verify:
		return "Verify"
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
	case Verify:
		return "check it is still true: baselines, A/B bisect, residuals against reality, regression scenarios"
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
type Arrangement struct {
	// Rail is a fixed-width column down the right, drawn full height. Empty
	// for a view that wants none.
	Rail   []string
	RailDp int

	// Rows divide everything left of the rail, top to bottom.
	Rows []Row
}

// Row is a horizontal band of one or more panels side by side.
type Row struct {
	// Weight is this row's share of the height, against the other rows.
	Weight int
	Cols   []Col
}

// Col is one panel in a row. WidthDp fixes it; zero takes what the fixed
// columns leave.
//
// Fixed is for a column of controls, whose fields have a width below which
// they stop being usable. Flexed is for the thing being read.
type Col struct {
	Name    string
	WidthDp int
}

// arrangementFor is the declared shape of each view. Changing a view's shape is
// changing this table.
func arrangementFor(v View) Arrangement {
	switch v {
	case Run:
		return withRail("Map", 340, "Schedule", "Scoreboard")
	case Debug:
		return withRail("Packet timeline", 340, "Inspector", "Link")
	case Verify:
		return withRail("Compare", 340, "Validate", "Scoreboard", "Regressions")
	case Bench:
		// Three columns and a strip, because comparing arms means reading a
		// table against the controls that produced it while the runs tick over
		// beside both. Sweep is fixed at a width its fields stay usable at;
		// Results takes the rest, being the thing actually read; Timelines gets
		// the full width under them because a burst is long and thin.
		return Arrangement{
			Rail:   []string{"Runs", "Experiment log", "Matrix"},
			RailDp: 460,
			Rows: []Row{
				{Weight: 3, Cols: []Col{{Name: "Sweep", WidthDp: 440}, {Name: "Results"}}},
				{Weight: 2, Cols: []Col{{Name: "Timelines"}}},
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
	return Arrangement{
		Rail: rail, RailDp: railDp,
		Rows: []Row{{Weight: 1, Cols: []Col{{Name: main}}}},
	}
}

// NumViews is how many views there are, for anything outside this package
// that has to enumerate them - a verb naming one, or a test covering all.
const NumViews = numViews

// PanelsIn is every panel a view shows, for anything outside this package that
// needs to know where a panel lives - showing one means showing its view.
func PanelsIn(v View) []string {
	a := arrangementFor(v)
	var out []string
	for _, r := range a.Rows {
		for _, c := range r.Cols {
			if c.Name != "" {
				out = append(out, c.Name)
			}
		}
	}
	return append(out, a.Rail...)
}
