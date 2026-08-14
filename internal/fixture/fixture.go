// Package fixture is the on-disk form of a whole setup: the network, where it
// is, and how it is being run.
//
// It lives here rather than inside the workbench because two very different
// programs read it - the desktop application, which opens one to resume work,
// and the headless test runner, which opens one to decide whether a firmware
// build still passes. A second definition of the same file would drift, and the
// drift would show up as a fixture that loads in one and not the other.
package fixture

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// Fixture is a saved study: the nodes, the boundary they were chosen from, the
// traffic to send, and what must be true afterwards.
//
// A saved network is the nodes alone, which loses the study around them.
// Reopening a piece of work should reopen the work, not the nodes it happened
// to contain.
type Fixture struct {
	Name    string          `json:"name"`
	Saved   time.Time       `json:"saved"`
	Nodes   []scenario.Node `json:"nodes"`
	Seed    uint64          `json:"seed"`
	FreqMHz float64         `json:"freq_mhz"`

	// Areas are the boundary's chosen regions, by name, with their polygons -
	// carried rather than re-fetched, so a fixture opens offline and gives the
	// same answer months later even if OSM's outline has moved.
	Areas    []Area  `json:"areas"`
	MarginKm float64 `json:"margin_km"`

	Sends      []Send      `json:"sends"`
	Assertions []Assertion `json:"assertions"`

	// View is where the map was looking, because reopening a study half a
	// country from where it was left is a small thing that feels wrong.
	Lat            float64 `json:"lat,omitempty"`
	Lon            float64 `json:"lon,omitempty"`
	MetresPerPixel float64 `json:"metres_per_pixel,omitempty"`
}

// Area is one chosen boundary and its polygons.
type Area struct {
	Name       string              `json:"name"`
	Boundaries []scenario.Boundary `json:"boundaries"`
}

// Send is one line typed at a node's console, once or on a repeat.
type Send struct {
	Node    string `json:"node"`
	AtMs    uint32 `json:"at_ms"`
	EveryMs uint32 `json:"every_ms"`
	Command string `json:"command"`
}

// Assertion is the saved form of a claim about a run. It is separate from
// engine.Assertion so that the file format can outlive a refactor of the
// checker, which is the usual reason a fixture stops loading.
type Assertion struct {
	Kind     string  `json:"kind"`
	Node     string  `json:"node,omitempty"`
	WithinMs uint32  `json:"within_ms,omitempty"`
	AtLeast  int     `json:"at_least,omitempty"`
	AtMost   int     `json:"at_most,omitempty"`
	MaxPct   float64 `json:"max_pct,omitempty"`
}

// Load reads a fixture from a path.
func Load(path string) (*Fixture, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Fixture
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s is not a fixture: %w", path, err)
	}
	if len(f.Nodes) == 0 {
		return nil, fmt.Errorf("%s has no nodes", path)
	}
	return &f, nil
}

// Checks converts the saved assertions into the ones the engine evaluates.
func (f *Fixture) Checks() []engine.Assertion {
	out := make([]engine.Assertion, 0, len(f.Assertions))
	for _, a := range f.Assertions {
		out = append(out, engine.Assertion{
			Kind: engine.AssertKind(a.Kind), Node: a.Node, WithinMs: a.WithinMs,
			AtLeast: a.AtLeast, AtMost: a.AtMost, MaxPct: a.MaxPct,
		})
	}
	return out
}

// RegionCommands is what a node must be told at boot for it to relay what the
// real one relays.
//
// Here rather than in the workbench because two programs issue it - the desktop
// application when it starts firmware, and the headless runner when it runs a
// fixture - and the region spelling is the trap this codebase has paid for
// twice. A region is written bare at the console, `region put sco`, while the
// key on the wire is a hash of the canonical "#sco". Two copies of that rule
// would eventually disagree, and the symptom is a mesh that transmits, relays
// nothing, and reports no error at all.
func RegionCommands(n scenario.Node) []string {
	if !n.Kind.Transmits() {
		return nil
	}
	var out []string
	// The wildcard first: it is the one line that makes a node relay something
	// it was never told about, and a reader of the log should meet it before
	// the specifics.
	if n.AllowAnyFlood {
		out = append(out, "region allowf *")
	}
	for _, r := range n.Regions {
		token := strings.TrimPrefix(r, "#")
		// put defines it, allowf permits flooding for it. A region that exists
		// but does not allow flooding relays nothing, which looks like a broken
		// mesh rather than a configuration choice.
		out = append(out, "region put "+token, "region allowf "+token)
	}
	if len(out) == 0 {
		return nil
	}
	// Without the save the map is gone at the next boot.
	out = append(out, "region save")
	if n.DefaultScope != "" {
		out = append(out, "region default "+strings.TrimPrefix(n.DefaultScope, "#"))
	}
	return out
}

// Permissive reports how many of the transmitting nodes forward flood traffic
// for any region, which is the one property of a fixture that changes what a
// result means. A caller that does not say so is quoting a more generous
// network than the real one.
func (f *Fixture) Permissive() (on, transmitting int) {
	for _, n := range f.Nodes {
		if !n.Kind.Transmits() {
			continue
		}
		transmitting++
		if n.AllowAnyFlood {
			on++
		}
	}
	return on, transmitting
}
