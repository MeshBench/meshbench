// The engine, behind the state layer.
//
// The old workbench built an engine and then read it from the frame loop. Here
// the store owns it: verbs ask it for things, the ticker advances it, and the
// renderer only ever sees a snapshot. That is the whole point of P0, and it is
// why the link margins below are computed once when the network changes rather
// than on every frame that draws them.
package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware/console"
	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
	"github.com/MeshBench/meshbench/internal/world/boundary"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Sim holds the engine and the scenario it was built from.
type Sim struct {
	// ui is whatever is drawing this session, if anything.
	ui UI
	// consoles is one scrollback per node, keyed by name.
	consoles map[string]*console.Buf
	// sendClock is when each scheduled send last fired, so a repeating one
	// repeats per interval rather than per tick.
	sendClock sendClock
	// statesMu guards states, which Reflash writes from its own goroutine and
	// the store goroutine reads to answer nodes.stats.
	statesMu sync.Mutex
	// logPath is where this run's full status log is being written, if
	// anywhere - set by whoever opened it, read by log.path so a script or a
	// menu action can find the file without knowing the naming scheme.
	logPath string
	// cold reports that the engine has been rebuilt and its link cache is
	// empty, so the next thing that can warm it should.
	cold bool
	// installFn puts a loaded network in place - the world's copy, the
	// engine, and the tick. Held here because opening a fixture and starting
	// a blank network are the same act with different contents, and a blank
	// one that took its own route would be a second kind of session with its
	// own set of things nobody remembered to set.
	installFn func(*state.Store, *state.World, Loaded, string)
	// servedAddrs is where each served companion can be reached, worked out
	// when it was served.
	servedAddrs map[string][]string
	// warmed reports that the matrix has been measured for the engine as it
	// stands. Cleared by a rebuild, set when a warm finishes uncancelled.
	warmed bool
	// warmHeld reports a warm that stopped to ask before spending bandwidth on
	// terrain. It is neither running nor finished, and it is a third state
	// rather than a second use of warmed because those are the two things the
	// rest of the system reads: nothing should wait on a measurement nobody is
	// doing, and nothing should be told the links have been measured when no
	// link has been.
	warmHeld bool
	// lastLiveProfiles is the engine's LiveProfiles() as of the last tick, so
	// the next tick can say how many pairs have been profiled since - the
	// diagnosis for a pause that is not the warming chip: some pair the last
	// warm measured is not the pair delivery just needed.
	lastLiveProfiles int64
	// lastPacketEvents is the engine's event count as of the last time an
	// open packet view was rebuilt, so the tick can skip the rebuild - a full
	// rescan of every event ever recorded - on a tick where nothing new
	// happened at all.
	lastPacketEvents int
	// compRev is the companion frame count as of the last time the view was
	// published, so a tick where nothing arrived skips the rebuild.
	compRev uint64
	// rfMode is which physics decides reception - "" or "calculated" for the
	// fast model, "waveform" for demodulator verdicts. See rfmode.go.
	rfMode string

	// unverifiedWiring runs boards nobody has watched boot yet.
	//
	// Off by default, and the default is the right one for a measurement: a
	// board whose wiring is wrong reports as silent rather than as mis-wired,
	// and a run that quietly did that would be worse than one that refused.
	//
	// But the import flow deliberately offers every board with wiring, on the
	// grounds that an image compiled for a board is the thing that establishes
	// whether that board works. Without this, that import could not then be
	// run, and the list of verified boards could never grow past what it
	// already holds. So the operator can lift the gate, and is told they have.
	unverifiedWiring bool
	// boardProbing is the single-flight guard on the capability matrix.
	//
	// Deliberately one at a time and never the whole fixture: a probe boots a
	// real board image under an emulator on top of the native nodes it talks
	// to, which is heavy enough that running the catalogue at once takes the
	// machine down rather than measuring anything.
	boardProbing bool
	// covCells is the operator's coverage-raster resolution - the long
	// edge, in cells - or zero for the default.
	covCells int

	// lastReadout throttles the interface readouts to human rate while the
	// run plays; the tick that paces the engine must not pay for tables.
	lastReadout time.Time
	// pace is the recent (wall, simulated) samples behind the transport's
	// x-realtime figure.
	pace []paceSample
	// realism is the RF Simulation imperfection switches. See rfmode.go.
	realism state.RFRealism
	// envDir is where the environment tiles live, or "" for bare earth.
	envDir string
	// envView is the store the map reads footprints from when no engine is
	// holding one open.
	envView environ.Provider
	// gpuWarm is whether the link matrix is measured on the GPU when one can
	// answer to the same accuracy. Off by default: it reads a rasterised
	// height grid rather than the DEM, which is the same answer on a county
	// and a different one on a continent.
	gpuWarm bool
	gpuMu   sync.Mutex
	// gpuOnce runs the probe itself exactly once. warm's own goroutine and
	// the startup gpu.state call can both reach for it on a fresh session;
	// this is what makes the second one wait for the first's answer instead
	// of reading gpuProbe before it exists.
	gpuOnce sync.Once

	// bench is the engine a running sweep cell owns, so the operator can watch
	// the run they started rather than a still clock. See benchlive.go.
	bench benchLive
	// gpuProbe is what asking this machine for a GPU answered, kept because
	// asking twice opens a device twice. Behind gpuMu once gpuOnce has run.
	gpuProbe *gpuProbe
	// gpuAsked records that the machine has been asked whether it has a GPU,
	// so the answer is not re-opened on every warm. Behind gpuMu.
	gpuAsked bool
	// tileCacheTiles overrides the tile cache bound, chosen in Configuration.
	tileCacheTiles int
	// prefs is what survives a restart, and persist is whether saving is on -
	// off in tests, on when the command has called LoadPrefs. prefsFile
	// overrides where it lives, which is how a test reads and writes real
	// settings without touching the developer's own.
	prefs     Prefs
	persist   bool
	prefsFile string
	// updateFeed points the release check somewhere other than the published
	// feed. Only a flag sets it, and every answer from a check says so.
	updateFeed string
	// movingCache reports a cache move in flight, so a second one cannot
	// start into the middle of the first; prefetching, the same for tiles.
	movingCache atomic.Bool
	prefetching atomic.Bool
	// geomFP fingerprints everything a path loss depends on, so a rebuild
	// can tell whether the measured matrix is still about this network.
	geomFP  uint64
	lastGPU GPUWarmResult
	// publishedNet is what the firmware catalogue offers, fetched once; nil
	// until the fetch has answered, empty after a fetch that failed.
	publishedNet      []publishedBuild
	fetchingPublished bool
	// imp is what has been fetched from a deployment but not yet applied.
	imp *importState
	// capturePath is where frames are being written, if anywhere.
	capturePath string
	// captureLive is the address frames are being streamed to for Wireshark,
	// if anywhere. Kept so the interface can say a live capture is running and
	// offer to stop it, rather than the operator having to remember.
	captureLive string
	// comps is one session per connected companion.
	comps map[string]*compSession
	// exp is the A/B matrix and what has come back from it.
	exp *experiment
	// feeding reports whether the live feed should keep pulling.
	feeding atomic.Bool
	// warmCancel stops the measurement in flight when the network changes
	// under it, guarded by warmMu.
	// prov is what every node is told at boot.
	prov       *Provisioning
	warmMu     sync.Mutex
	warmCancel context.CancelFunc
	// areas is the accepted study area, as boundaries.
	areas []scenario.Boundary
	// foundAreas is the last search's matches, awaiting a choice.
	foundAreas []boundary.Found

	// freqMHz and seed are what the current engine was built with, so a
	// rebuild reproduces it rather than guessing.
	freqMHz float64
	seed    uint64
	// excessLossDB is the calibration term: everything the bare-earth model
	// does not contain - vegetation, buildings, the ground itself not being a
	// knife edge.
	excessLossDB float64
	// excessSet distinguishes "nobody has said" from "somebody said zero",
	// which are different answers and only one of them is a default.
	excessSet bool

	eng      *engine.Engine
	nodes    []scenario.Node
	terr     propagation.Terrain
	starting atomic.Bool
	cpu      *cpuSampler
	history  *nodeHistory
	states   map[string]string
	// servedMu guards served and servedAddrs together, since a listener and
	// its reachable addresses are one fact told across two maps. Close runs
	// from a goroutine that is not the store's, so without this a verb
	// handler serving or dropping a companion at the same moment is a
	// concurrent map write the Go runtime kills the process for.
	servedMu sync.Mutex
	served   map[string]*engine.CompanionLink

	// domainState holds the per-domain state that used to live as a typed field
	// here, for the domains split out of session that cannot name their own
	// type on this struct without an import cycle. Keyed by a name the domain
	// chooses; reached through DomainState. Guarded by domainStateMu.
	domainState   map[string]any
	domainStateMu sync.Mutex
}

// terrainStore is the elevation the engine sees.
//
// The same on-disk cache the rest of the tool fills, and offline: a path loss
// computed while a tile downloads is a path loss nobody asked for at a moment
// nobody chose. Missing tiles answer "no data", which the engine already
// handles - it is bare earth for that profile and says so.
// terrainCached is terrain that answers only from the tile cache - for the
// callers that must not block on a download, where a missing tile is an
// honest gap rather than a wait.
func (s *Sim) terrainCached() propagation.Terrain {
	t := s.terrain()
	if ts, ok := t.(*terrain.TileStore); ok {
		return cachedOnly{ts}
	}
	return t
}

type cachedOnly struct{ ts *terrain.TileStore }

func (c cachedOnly) ElevationM(lat, lon float64) (float64, bool) {
	return c.ts.ElevationCachedM(lat, lon)
}

func (s *Sim) terrain() propagation.Terrain {
	if s.terr != nil {
		return s.terr
	}
	dir := s.tileCacheDir()
	if dir == "" {
		s.terr = bareEarth{}
		return s.terr
	}
	st, err := terrain.NewTileStore(dir)
	if err != nil {
		s.terr = bareEarth{}
		return s.terr
	}
	st.Zoom = terrain.DefaultZoom
	if s.tileCacheTiles > 0 {
		st.MaxLoadedTiles = s.tileCacheTiles
	}
	// Off until somebody says otherwise. The lazy fetch inside a profile walk
	// is the path that spent half a gigabyte unasked, and gating only the
	// announced prefetch would have left it doing exactly that, one tile at a
	// time, from the middle of a measurement.
	st.Offline = !s.terrainAllowed()
	s.terr = st
	return s.terr
}

// bareEarth answers for nowhere, which is not the same as answering zero: the
// engine treats "no data" as a profile it cannot use rather than as sea level
// across the Atlantic.
type bareEarth struct{}

func (bareEarth) ElevationM(float64, float64) (float64, bool) { return 0, false }

// build makes an engine for a set of nodes.
func (s *Sim) build(nodes []scenario.Node, freqMHz float64) {
	s.buildSeeded(nodes, freqMHz, defaultSeed)
}

// DefaultExcessLossDB is what the bare-earth model is missing.
//
// The diffraction calculation is sound - measured against the DEM it charges
// +47 dB for a 326 m ridge and exactly zero for a clear path - but it models
// bare earth. It has no trees, no buildings and no ground that is anything but
// a knife edge, so paths that cross a ridge close in the simulator that do not
// close on ScotMesh: The Mysterons reached Leslie, Cadham and Bishop Hill
// through the Lomond Hills, which is not possible and was reported as such.
//
// It is now a measurement rather than the guess it started as, three times
// over. The first fit - 118 Fife receptions - put it at 20.4 dB. The second,
// after the matching pipeline learned to pair an observation with the link its
// SNR was actually measured on, said 23.5 across the whole of ScotMesh. The
// third is the converged figure: with predictions past the modem's +15 dB
// reporting ceiling censored out of the fit - they can only say "at least",
// and a bound must not vote a number - repeated fetch-then-calibrate rounds
// against 444 live nodes settle at 25.1 dB after one step and stay there,
// fitted on the 357 observations of 1,363 whose predictions the register
// could actually express.
//
// This constant is what a network nobody has observed gets, which is why it
// carries the whole-country figure rather than a per-network one: it is a
// clutter-and-multipath term for UK terrain at 869 MHz, not a ScotMesh
// setting. A network with observations of its own refines it through
// validate.fetch then validate.calibrate, which re-derives the total from
// whatever is current and now converges instead of creeping.
//
// Buildings do not replace it, which was the open question, and they do not
// dent it either. Fitted again against 455 ScotMesh nodes with 4.5 million
// real footprints loaded, the term comes out 0.04 dB HIGHER than the same fit
// over bare earth - 29.515 against 29.475 - while the footprints remove 27.0%
// of the link matrix. Loading an environment changes what the model says about
// a town a great deal and changes this constant, which is what a session
// without one gets, not at all.
// docs/studies/excess-loss-buildings-saturated.md is the record.
//
// Those nights' bare-earth fits converge to 29.5 and 29.8 dB rather than 25.1,
// on around 1,200 voting observations against the 357 behind the figure below.
// One evening is not what moved this number the last three times and two do
// not move it now, particularly when they disagree by 0.3 dB; what would is
// that protocol repeated across several days.
//
// Studies comparing two firmware builds are unaffected in direction, because
// both arms carry the same term.
const DefaultExcessLossDB = 25.1

// defaultSeed is the one a fresh session starts from. Fixed, because a
// simulator whose default run differs every time cannot be used to show
// anybody a result.
const defaultSeed = 9001

// SetLogPath records where this run's full status log is being written, for
// log.path to report.
func (s *Sim) SetLogPath(path string) { s.logPath = path }

// buildSeeded is build, with the draw stated.
func (s *Sim) buildSeeded(nodes []scenario.Node, freqMHz float64, seed uint64) {
	// A rebuild that changes nothing geometric keeps the measured matrix.
	//
	// Reset, a new seed, and switching run kinds all remake the engine, and
	// the old workbench re-measures every pair each time - fast only because
	// its tiles are hot. A path loss depends on where the nodes are, their
	// heights and powers, the frequency and the excess loss, and on nothing
	// else the rebuild changes; if that whole fingerprint is unchanged, the
	// matrix is the same matrix.
	var carried map[[2]int]float64
	fp := geometryFingerprint(nodes, freqMHz, s.envDir)
	if s.eng != nil && fp == s.geomFP {
		carried = s.eng.LinkCacheSnapshot()
	}
	if carried == nil {
		// Nothing in this process, but perhaps on disk: a matrix measured in
		// a previous launch for this exact geometry is this geometry's
		// matrix, and reading it is a file open instead of a warm.
		carried = loadMatrix(s.matrixDir(), fp)
	}
	// Cold from this moment. An engine is rebuilt rather than rewound, and a
	// new one carries no link cache: the old workbench warms on every rebuild
	// for exactly this reason, and its comment is worth repeating - an empty
	// cache bills its terrain profiles to whoever sends the first message,
	// which reads as "runs fine until I send, then stuck".
	s.cold = true
	// Under the lock, because the warm that is very likely still running sets
	// this from its own goroutine as it finishes. A rebuild writing it bare
	// was two goroutines on one bool, which the race detector calls what it
	// is and which decides nothing reliably: "warming up" is what play refuses
	// to start in front of.
	s.warmMu.Lock()
	s.warmed, s.warmHeld = false, false
	s.warmMu.Unlock()
	s.geomFP = fp
	defer func() {
		if carried != nil && s.eng != nil {
			// Primes the cache; does not claim the matrix is complete. A
			// carried map can be a partial in-process snapshot - radio-state
			// changes evict entries live once firmware reports, and a disk
			// matrix was only ever complete against the baseline import
			// figures a firmware node's real configuration can diverge from.
			// s.cold and s.warmed are left as set above, so the warm every
			// call site already runs still happens - cheap now, since most
			// of it lands on this primed cache, but the thing that gets to
			// say every pair has actually been measured.
			s.eng.RestoreLinkCache(carried)
		}
	}()
	if s.eng != nil {
		_ = s.eng.Close()
	}
	s.nodes = nodes
	s.freqMHz = freqMHz
	s.seed = seed
	if !s.excessSet {
		s.excessLossDB = DefaultExcessLossDB
	}
	s.eng = engine.New(s.terrain(), engine.Config{
		FreqMHz: freqMHz, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
		ExcessPathLossDB: s.excessLossDB,
		RFMode:           rfModeOf(s.rfMode),
		UnverifiedWiring: s.unverifiedWiring,
		Realism:          engineRealism(s.realism),
	})
	for _, n := range nodes {
		s.eng.Add(n, nil)
	}
	if s.envDir != "" {
		s.eng.Env = environ.OpenTiles(s.envDir)
	}
}

// linksOf is every pair that can hear each other, with the weaker direction's
// margin, from the engine's own path loss.
//
// n squared, and on the 311 node fixture that is 48,000 path losses - which is
// why it is measured once, off the store's goroutine, and lands in the
// snapshot, rather than being something the map does while drawing.
//
// The engine and its nodes are arguments rather than the session's own fields
// because the only caller is a warm, which runs on its own goroutine and
// outlives the network it was started for. Reading the live pair meant
// measuring whichever network had been opened since - and during a rebuild
// those two disagree by construction, the node list being replaced in one
// assignment while the new engine is filled a node at a time after it. An
// index off the list was then out of range in the engine, which is a bounds
// panic on a worker and the whole process with it. Handed both, this can only
// index the slice it was given the length of.
func linksOf(eng *engine.Engine, nodes []scenario.Node) []state.Link {
	if eng == nil {
		return nil
	}
	var out []state.Link
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			loss, ok := eng.PathLossForTest(i, j)
			if !ok {
				continue
			}
			m := linkbudget.MarginDB(nodes[i], nodes[j], loss)
			// A link that does not close in the weaker direction by a wide
			// margin is not a link anybody wants drawn: below -20 dB the pair
			// is a different part of the country.
			if m < -20 {
				continue
			}
			out = append(out, state.Link{
				A: i, B: j, MarginDB: m, Known: true,
				AtoB: linkbudget.OneWayDB(nodes[i], nodes[j], loss),
				BtoA: linkbudget.OneWayDB(nodes[j], nodes[i], loss),
			})
		}
	}
	return out
}
