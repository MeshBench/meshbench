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
		return "check it is still true: baselines, A/B bisect, residuals against reality"
	case Bench:
		return "compare configurations: sweep a parameter, repeat it, read what differed"
	case App:
		return "write a client against it: an endpoint, the protocol decoded, faults on a button"
	default:
		return "build and site: import, place, drag, boundary, coverage"
	}
}

// layoutFor is the declared arrangement of each view: which panels, and how
// the space is divided. Changing a view's shape is changing this table.
func layoutFor(v View) (main string, side []string) {
	switch v {
	case Run:
		return "Map", []string{"Schedule", "Scoreboard"}
	case Debug:
		return "Packet timeline", []string{"Inspector", "Link"}
	case Verify:
		return "Compare", []string{"Validate", "Scoreboard"}
	case Bench:
		return "Runs", []string{"Sweep", "Matrix"}
	case App:
		return "Companion bench", []string{"Events", "Console"}
	default:
		return "Map", []string{"Nodes", "Inspector"}
	}
}

// NumViews is how many views there are, for anything outside this package
// that has to enumerate them - a verb naming one, or a test covering all.
const NumViews = numViews
