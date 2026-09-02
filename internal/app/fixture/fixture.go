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

	embedded "github.com/MeshBench/meshbench/fixtures"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Format is the fixture format this build writes, and the highest one it will
// read.
//
// It moves when a file this build writes would be *misread* by the build
// before it, which is not the same as changed. Adding a field an older build
// ignores costs that build nothing, so it does not move the number; a field
// changing meaning, or one becoming load-bearing, does.
//
// 1 is the format as it stood when the number was introduced. A file written
// before that carries no number at all, reads as 0, and is still read: the
// shape did not change on the day it started being declared.
const Format = 1

// Fixture is a saved study: the nodes, the boundary they were chosen from, the
// traffic to send, and what must be true afterwards.
//
// A saved network is the nodes alone, which loses the study around them.
// Reopening a piece of work should reopen the work, not the nodes it happened
// to contain.
type Fixture struct {
	// Format is what the writing build called its own file layout. Carried at
	// the top so that a reader who opens the file in an editor meets it before
	// anything it governs.
	Format int `json:"format,omitempty"`

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

// Load reads a fixture by path or by name.
//
// A path is read as given. A name goes through Find, which looks where an
// installed copy keeps its fixtures and then inside the binary - see find.go
// for why that is not optional.
func Load(path string) (*Fixture, error) {
	b, err := read(path)
	if err != nil {
		return nil, err
	}
	var f Fixture
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s is not a fixture: %w", path, err)
	}
	if err := readable(path, f.Format); err != nil {
		return nil, err
	}
	if len(f.Nodes) == 0 {
		return nil, fmt.Errorf("%s has no nodes", path)
	}
	return &f, nil
}

// readable refuses a file from the future, and says which build wrote it and
// which one is refusing.
//
// Refusing rather than reading what it recognises and ignoring the rest. A
// fixture is the input to a simulation, and a simulation run on three quarters
// of its inputs does not fail: it produces a plausible answer about a network
// nobody described, which is the worst thing this project can do. Refusing
// costs a person one upgrade; misreading costs them a result they believed.
//
// Before the number check, because it comes before the nodes check in Load: a
// later format may well have stopped spelling the nodes this way, and "has no
// nodes" would send the reader looking for a fault in their own file.
func readable(path string, format int) error {
	if format <= Format {
		return nil
	}
	return fmt.Errorf("%s is fixture format %d and this build (MeshBench %s) "+
		"reads up to format %d. It was written by a later MeshBench: install "+
		"that release to open it", path, format, version.String(), Format)
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

// read resolves a fixture and returns its bytes, from disk or from the copy
// compiled in.
func read(nameOrPath string) ([]byte, error) {
	where, isEmbedded, err := Find(nameOrPath)
	if err != nil {
		return nil, err
	}
	if isEmbedded {
		return embedded.FS.ReadFile(where)
	}
	return os.ReadFile(where)
}
