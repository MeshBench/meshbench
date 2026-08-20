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
	"github.com/MeshBench/meshbench/internal/mesh/console"
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
	// warmed reports that the matrix has been measured for the engine as it
	// stands. Cleared by a rebuild, set when a warm finishes uncancelled.
	warmed bool
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
	// sdrServers is every node currently exposed as an rtl_tcp source,
	// with the sample rate its stream was attached at.
	sdrServers map[string]*sdrServer
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
	// off in tests, on when the command has called LoadPrefs.
	prefs   Prefs
	persist bool
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
	served   map[string]*engine.CompanionLink
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
	fp := geometryFingerprint(nodes, freqMHz)
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
	s.cold, s.warmed = true, false
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
		Realism:          engineRealism(s.realism),
	})
	for _, n := range nodes {
		s.eng.Add(n, nil)
	}
	if s.envDir != "" {
		s.eng.Env = environ.OpenTiles(s.envDir)
	}
}

// links is every pair that can hear each other, with the weaker direction's
// margin, from the engine's own path loss.
//
// n squared, and on the 311 node fixture that is 48,000 path losses - which is
// why it is a verb that runs once on the store's goroutine and lands in the
// snapshot, rather than something the map does while drawing.
func (s *Sim) links() []state.Link {
	if s.eng == nil {
		return nil
	}
	var out []state.Link
	for i := range s.nodes {
		for j := i + 1; j < len(s.nodes); j++ {
			loss, ok := s.eng.PathLossForTest(i, j)
			if !ok {
				continue
			}
			m := linkbudget.MarginDB(s.nodes[i], s.nodes[j], loss)
			// A link that does not close in the weaker direction by a wide
			// margin is not a link anybody wants drawn: below -20 dB the pair
			// is a different part of the country.
			if m < -20 {
				continue
			}
			out = append(out, state.Link{
				A: i, B: j, MarginDB: m, Known: true,
				AtoB: linkbudget.OneWayDB(s.nodes[i], s.nodes[j], loss),
				BtoA: linkbudget.OneWayDB(s.nodes[j], s.nodes[i], loss),
			})
		}
	}
	return out
}
