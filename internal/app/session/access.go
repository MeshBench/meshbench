package session

import (
	"context"
	"io"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware/console"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
	"github.com/MeshBench/meshbench/internal/study/pathview"
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

// TerrainDownloadsOn reports whether terrain may be downloaded.
//
// One answer rather than the three states this used to carry. Nobody is asked
// any more: downloads are on unless somebody turned them off, so "refused" and
// "not yet answered" are the same thing to everything downstream.
func (s *Sim) TerrainDownloadsOn() bool {
	return s.terrainAllowed()
}

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

// SavePrefs persists the preferences, saying into the world when it cannot.
func (s *Sim) SavePrefs(w *state.World) error { return s.savePrefs(w) }

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

// NamedNum is NumField for a field that has to be named to be meant: no bare
// number fills it, because the caller spent their bare value on the verb's one
// primary parameter.
func NamedNum(p any, name string) (float64, bool) { return namedNum(p, name) }

// FinishJob and SoleString are two more shared readers a split-out domain's
// handlers need: FinishJob removes a completed job from the list, SoleString
// reads a verb's whole argument as a string.
func FinishJob(jobs []state.Job, id string) []state.Job { return finishJob(jobs, id) }
func SoleString(p any) string                           { return soleString(p) }

// PrimaryString reads a verb's documented primary string parameter by name,
// falling back to a lone unnamed value. What a split-out domain's handler uses
// wherever its description names a primary parameter, so that the name it
// publishes is the name it reads.
func PrimaryString(p any, name string) string { return primaryString(p, name) }

// NamedField reads a named (non-primary) string parameter; BadParams builds the
// coded error a verb returns for a bad argument; StateNodes is the scenario in
// the snapshot's own node form. Three more shared readers the split-out domains
// need, thin exports of what every verb here already uses.
func NamedField(p any, name string) (string, bool)  { return namedField(p, name) }
func BadParams(format string, args ...any) error    { return badParams(format, args...) }
func StateNodes(nodes []scenario.Node) []state.Node { return stateNodes(nodes) }

// AreaOf is the one way a set of boundaries becomes a study area, rings and
// holes together, for the boundary domain as well as for a fixture load.
func AreaOf(name string, boundaries []scenario.Boundary) state.Area {
	return areaOf(name, boundaries)
}

// FindNode looks a node up in the snapshot by name, for the split-out domains.
func FindNode(nodes []state.Node, name string) (state.Node, bool) { return findNode(nodes, name) }

// NodeIsEmulated reports whether a named node runs under an emulator, and
// NodeIndex finds a node's position in the scenario - shared node readers the
// split-out node verbs need.
func (s *Sim) NodeIsEmulated(w *state.World, name string) (bool, error) {
	return s.nodeIsEmulated(w, name)
}
func (s *Sim) NodeIndex(name string) (int, bool) { return s.nodeIndex(name) }

// BoardProbing is the single-flight guard on the capability matrix, and
// LiveEngine is the engine a running sweep cell owns, or the main one - the two
// the board verbs need.
func (s *Sim) BoardProbing() bool         { return s.boardProbing }
func (s *Sim) SetBoardProbing(v bool)     { s.boardProbing = v }
func (s *Sim) LiveEngine() *engine.Engine { return s.liveEngine() }

// ProfileFor computes a link's terrain profile in both directions, publishing
// it to the store - shared by the link.profile verb and the link budget.
func (s *Sim) ProfileFor(st *state.Store, from, to string, atob, btoa float64) {
	s.profileFor(st, from, to, atob, btoa)
}

// StateProfile renders a cut-through into a snapshot Profile, and TermsOf turns
// link-budget terms into their snapshot form - the two link-budget helpers the
// link.pair verb shares with core.
func StateProfile(cut pathview.CutThrough, from, to string, atob, btoa, aAGL, bAGL, freq float64) *state.Profile {
	return stateProfile(cut, from, to, atob, btoa, aAGL, bAGL, freq)
}
func TermsOf(in []linkbudget.Term) []state.BudgetTerm { return termsOf(in) }

// BoolField reads a named boolean parameter; NoSuchNode is the standard
// not-found error; IsCompanionNode reports whether a named node is a companion.
// Three more shared readers the split-out domains need.
func BoolField(p any, name string) (bool, bool) { return boolField(p, name) }
func NoSuchNode(name string) error              { return noSuchNode(name) }

// WrongCallback is the refusal an internal verb returns when it is handed
// something other than the value its own worker passes it, for the callbacks
// that live in the split-out domains.
func WrongCallback(verb string) error { return wrongCallback(verb) }

// NumAsked, NumInRange and UnknownNames are the refuse-rather-than-default
// readers, for the split-out domains: NumAsked separates "not given" from
// "given and unusable", NumInRange adds a documented default and a range, and
// UnknownNames turns names this network has not got into a refusal that offers
// the ones it has. NameList is the few-and-a-count form those refusals use so a
// message about a typo does not print three hundred node names.
func NumAsked(verb, name string, p any) (float64, bool, error) {
	return numAsked(verb, name, p)
}
func NumInRange(verb, name string, p any, def, lo, hi float64) (float64, error) {
	return numInRange(verb, name, p, def, lo, hi)
}

// NamedNumAsked and NamedNumInRange are the same two readers for a field that
// has to be named to be meant, so a verb whose primary parameter is not a
// number does not read its bare argument as one.
func NamedNumAsked(verb, name string, p any) (float64, bool, error) {
	return namedNumAsked(verb, name, p)
}
func NamedNumInRange(verb, name string, p any, def, lo, hi float64) (float64, error) {
	return namedNumInRange(verb, name, p, def, lo, hi)
}
func UnknownNames(verb string, nodes []state.Node, names []string) error {
	return unknownNames(verb, nodes, names)
}
func NameList(names []string) string { return nameList(names) }

// CapturePath and CaptureLive are where frames are being written and streamed,
// if anywhere; the capture verbs set and clear them.
func (s *Sim) CapturePath() string     { return s.capturePath }
func (s *Sim) SetCapturePath(p string) { s.capturePath = p }
func (s *Sim) CaptureLive() string     { return s.captureLive }
func (s *Sim) SetCaptureLive(a string) { s.captureLive = a }

// Executable reports whether a path is an executable file, per platform - the
// reader the capture package's dumpcap search needs.
func Executable(path string) bool                             { return executable(path) }
func IsCompanionNode(nodes []scenario.Node, name string) bool { return isCompanionNode(nodes, name) }

// Provisioning is the current provisioning settings, created from the default
// on first use; the provisioning verbs read and mutate it through this pointer.
// The type and its logic stay in core because the experiment matrix shares them.
func (s *Sim) Provisioning() *Provisioning { return s.provisioning() }

// Finishing is the context a verb uses to report how a job ended even as the
// job's own context is being cancelled - the shared helper the split-out
// domains' long-running pulls need.
func Finishing(ctx context.Context) (context.Context, context.CancelFunc) { return finishing(ctx) }

// ConsoleFor is the console buffer for a named node, creating it if need be -
// the shared reader the fleet and radio verbs use to talk to a node.
func (s *Sim) ConsoleFor(name string) (*console.Buf, error) { return s.consoleFor(name) }

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

// BenchTake publishes a cell's own engine as the live one, and passing nil
// hands the view back to the session's engine.
//
// It is here so the experiment package can show the run somebody started: a
// cell builds an engine of its own, with its own storage, and without this the
// workbench draws a clock that does not advance for as long as the matrix takes.
func (s *Sim) BenchTake(e *engine.Engine) { s.bench.take(e) }

// ProvisionLinesFor is the session's own provisioning for a node with an arm's
// settings written over it.
//
// Exported for the experiment package, and load-bearing there: a cell that
// sends the defaults instead compares two arms that were both configured the
// same way, reports no difference, and looks like a clean result.
func (s *Sim) ProvisionLinesFor(n scenario.Node, arm ExpArm) []state.ProvisionLine {
	return s.provisionLinesFor(n, arm)
}

// NewCompanionSink accepts one companion's output on behalf of a caller that
// drives the node through the companion protocol but has no use for what the
// frames decoded to.
//
// A claim has to be held for a node to be driven at all, which is the whole of
// what an experiment's cell wants from a companion session: the decoded self
// info, contacts and messages are the panels' business, not the matrix's.
func (s *Sim) NewCompanionSink(node string) io.Writer { return &compSession{node: node} }

// ScenarioEpoch is the instant a scenario's clock starts from, which a run has
// to share for two cells to be comparable at all.
const ScenarioEpoch = scenarioEpoch
