package session

import (
	"context"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/boundary"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The Sim's state, reachable from the domain packages split out of this one.
//
// A domain package - study, and the ones that follow - holds verbs that read
// and drive the running simulation, but the Sim's fields stay unexported so the
// package that owns them stays the only thing that can write them by hand. These
// are the read accessors and the driving methods those packages are allowed, and
// no more: everything a split-out verb legitimately needs, said once here rather
// than by exporting the fields.

// Nodes is the scenario as it stands.
func (s *Sim) Nodes() []scenario.Node { return s.nodes }

// Engine is the running engine, or nil before one is built.
func (s *Sim) Engine() *engine.Engine { return s.eng }

// FreqMHz, ExcessLossDB and the environment are what the current engine was
// built with.
func (s *Sim) FreqMHz() float64      { return s.freqMHz }
func (s *Sim) ExcessLossDB() float64 { return s.excessLossDB }
func (s *Sim) ExcessSet() bool       { return s.excessSet }
func (s *Sim) EnvDir() string        { return s.envDir }

// GPUWarm reports whether the link matrix is measured on the GPU.
func (s *Sim) GPUWarm() bool { return s.gpuWarm }

// CovCells is the operator's coverage-raster resolution, or zero for default.
func (s *Sim) CovCells() int { return s.covCells }

// Prefs is the loaded preferences.
func (s *Sim) Prefs() Prefs { return s.prefs }

// ImportURL is where the pending import came from, or empty when none.
func (s *Sim) ImportURL() string {
	if s.imp == nil {
		return ""
	}
	return s.imp.url
}

// Terrain and TerrainCached are the profile source: Terrain fetches, and
// TerrainCached refuses to.
func (s *Sim) Terrain() propagation.Terrain       { return s.terrain() }
func (s *Sim) TerrainCached() propagation.Terrain { return s.terrainCached() }

// Hillshade rasters the relief over a box.
func (s *Sim) Hillshade(south, north, west, east float64) (*state.Coverage, error) {
	return s.hillshade(south, north, west, east)
}

// Capture is the waterfall at a node, once the run has traffic.
func (s *Sim) Capture(ctx context.Context, at int) (*state.Coverage, string) {
	return s.capture(ctx, at)
}

// ResidualsOf is the observed-against-predicted comparison.
func (s *Sim) ResidualsOf(obs []state.Observed, links []state.Link, nodes []state.Node) *state.Residuals {
	return s.residualsOf(obs, links, nodes)
}

// Warm measures the link matrix for the engine as it stands.
func (s *Sim) Warm(st *state.Store, nodes int) { s.warm(st, nodes) }

// Rebuild rebuilds the engine at the current seed.
func (s *Sim) Rebuild(w *state.World) error { return s.rebuild(w) }

// SavePrefs persists the preferences.
func (s *Sim) SavePrefs() { s.savePrefs() }

// RoutesBetween is the path a message would take from one node to another.
func (s *Sim) RoutesBetween(from, to string) ([]state.Route, error) {
	return s.routesBetween(from, to)
}

// Areas is the accepted study area as boundary geometry; FoundAreas is the last
// place-search's matches awaiting a choice. The boundary verbs read and replace
// both, and the fields stay unexported so only the accessor writes them.
func (s *Sim) Areas() []scenario.Boundary       { return s.areas }
func (s *Sim) SetAreas(a []scenario.Boundary)   { s.areas = a }
func (s *Sim) FoundAreas() []boundary.Found     { return s.foundAreas }
func (s *Sim) SetFoundAreas(f []boundary.Found) { s.foundAreas = f }

// Seed is the run seed the current scenario was built at.
func (s *Sim) Seed() uint64 { return s.seed }

// BuildSeeded rebuilds the engine over a node set at a frequency and seed, for
// the boundary verbs that prune the scenario back to the study area.
func (s *Sim) BuildSeeded(nodes []scenario.Node, freqMHz float64, seed uint64) {
	s.buildSeeded(nodes, freqMHz, seed)
}

// domainRegistrars are the verb sets that live in the domain packages split out
// of this one. A domain registers itself from its own init; Register runs them
// after the core verbs. This is how a domain package hangs its verbs off the
// store without the store's package having to import it - which it cannot,
// since the domain imports the store's Sim.
var domainRegistrars []func(*state.Store, *Sim)

// RegisterDomain adds a split-out domain's verbs. Called from a domain
// package's init; a caller that wants those verbs blank-imports the package.
func RegisterDomain(f func(*state.Store, *Sim)) {
	domainRegistrars = append(domainRegistrars, f)
}

func runDomainRegistrars(st *state.Store, s *Sim) {
	for _, f := range domainRegistrars {
		f(st, s)
	}
}

// NumField and StringField read a parameter from a verb's argument, for the
// handlers that live in the split-out domain packages. Thin exports of the
// unexported readers every verb here already uses, so the two cannot diverge.
func NumField(p any, name string) (float64, bool)   { return numField(p, name) }
func StringField(p any, name string) (string, bool) { return stringField(p, name) }

// FinishJob and SoleString are two more shared readers a split-out domain's
// handlers need: FinishJob removes a completed job from the list, SoleString
// reads a verb's whole argument as a string.
func FinishJob(jobs []state.Job, id string) []state.Job { return finishJob(jobs, id) }
func SoleString(p any) string                           { return soleString(p) }

// NamedField reads a named (non-primary) string parameter; BadParams builds the
// coded error a verb returns for a bad argument; StateNodes is the scenario in
// the snapshot's own node form. Three more shared readers the split-out domains
// need, thin exports of what every verb here already uses.
func NamedField(p any, name string) (string, bool)  { return namedField(p, name) }
func BadParams(format string, args ...any) error    { return badParams(format, args...) }
func StateNodes(nodes []scenario.Node) []state.Node { return stateNodes(nodes) }

// Finishing is the context a verb uses to report how a job ended even as the
// job's own context is being cancelled - the shared helper the split-out
// domains' long-running pulls need.
func Finishing(ctx context.Context) (context.Context, context.CancelFunc) { return finishing(ctx) }

// SetCoverageCells records the operator's coverage-raster resolution, on the
// Sim and in the preferences both, for the study verbs that let it be chosen.
func (s *Sim) SetCoverageCells(n int) {
	s.covCells = n
	s.prefs.CoverageCells = n
}

// SetExcessLoss records the calibration term - the decibels the bare-earth
// model is short by - and whether it was set by hand rather than assumed.
func (s *Sim) SetExcessLoss(db float64, set bool) {
	s.excessLossDB, s.excessSet = db, set
}
