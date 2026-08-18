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
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/boundary"
	"github.com/MeshBench/meshbench/internal/console"
	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/linkbudget"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/sdr"
	"github.com/MeshBench/meshbench/internal/terrain"
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
	// sdrServers is every node currently exposed as an rtl_tcp source.
	sdrServers map[string]*sdr.RTLTCP
	// realism is the RF Simulation imperfection switches. See rfmode.go.
	realism state.RFRealism
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
	terr     coverage.Terrain
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
func (s *Sim) terrain() coverage.Terrain {
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
// It is now a measurement rather than the guess it started as. Fitted against
// 118 real receptions from the live ScotMesh deployment - packets carrying an
// SNR from an observer whose public key matches a node in the scenario - the
// median residual is +20.4 dB, positive meaning the model predicted more
// signal than was heard.
//
// The value was chosen before that as "what it takes for the three impossible
// links across the Lomond ridge to fail", and it landed within half a decibel
// of what the network says. That is luck as much as judgement, and the reason
// the fit exists is so nobody has to rely on it again: validate.fetch then
// validate.calibrate re-derives it from whatever observations are current.
//
// Studies comparing two firmware builds are unaffected in direction, because
// both arms carry the same term.
const DefaultExcessLossDB = 20

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
	fp := geometryFingerprint(nodes, freqMHz, s.excessLossDB)
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

// snapshotNodes copies the nodes themselves, so a worker reading them cannot
// be racing a verb that writes them.
//
// Element-wise, which is what the reported race needed: the fields written
// while a warm is in flight are scalars and strings on the node itself. A
// node's Regions slice is still shared with the original, and deliberately -
// every writer replaces that slice wholesale rather than editing it in place,
// so the copy keeps whichever one it was given.
func snapshotNodes(in []scenario.Node) []scenario.Node {
	return append([]scenario.Node(nil), in...)
}

// warm computes the link margins on a worker and hands them to the store.
//
// One at a time: a second warm while the first is running would compute the
// same thing twice and race to publish it.
func (s *Sim) warm(st *state.Store, nodes int) {
	if s.eng == nil {
		return
	}
	// Cancel whatever is running and start again.
	//
	// Marking it stale and repeating afterwards was not enough: a warm holds
	// no copy of the engine, so one started for a 58-node fixture carried on
	// against the 676-node import that replaced it - 228,000 terrain profiles
	// for a network nobody is looking at, while the 44 nodes that were
	// actually on screen showed no links at all and nothing said why.
	s.warmMu.Lock()
	if s.warmCancel != nil {
		s.warmCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.warmCancel = cancel
	s.warmMu.Unlock()

	s.cold = false
	// The network this warm is for, taken now. Everything below runs after
	// the verb that started it has returned, and s.eng/s.nodes belong to
	// whatever has been opened since.
	//
	// A copy of the nodes, not of the slice header. That distinction was the
	// whole of a data race: assigning s.nodes shares the backing array, so a
	// warm reading a node's fields on its worker and a verb writing them on
	// the store goroutine - setFirmware, say - are touching the same memory.
	// The intent here was always a snapshot; this is what makes it one.
	eng, warmNodes, freqMHz := s.eng, snapshotNodes(s.nodes), s.freqMHz
	go func() {
		defer cancel()
		total := nodes * (nodes - 1) / 2
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "links", What: "measuring every link", Total: total})

		// On by default where there is hardware for it, decided once.
		s.gpuDefault()
		// The GPU first, if it is switched on and can answer honestly. What
		// it fills, the cores below no longer have to: WarmLinks asks the
		// cache before it measures anything.
		if s.gpuWarm {
			res := s.warmOnGPU(eng, warmNodes, freqMHz, func(what string, done, total int) {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "links", What: what, Done: done, Total: total})
			})
			s.gpuMu.Lock()
			s.lastGPU = res
			s.gpuMu.Unlock()
			if res.Used {
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: "links", What: "measuring every link on the GPU",
					Done: res.Pairs, Total: total})
			} else if res.Why != "" {
				// Said aloud. A silent fall back is a status frozen on the
				// last phase while every core spikes, which reads as a hang.
				_, _ = st.Do(ctx, "ui.said",
					"the GPU declined this one - "+res.Why+" - measuring on the processor")
			}
			_, _ = st.Do(ctx, "gpu.state", nil)
		}

		eng.WarmLinks(ctx, func(done, of int) {
			// No second throttle here: the engine already reports every 512th
			// pair, and a filter stacked on a filter only let through their
			// common multiples - the first status update came at pair 32,000,
			// which on the processor is most of the warm spent looking hung.
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "links", What: "measuring every link on the processor",
				Done: done, Total: of})
		})

		if ctx.Err() != nil {
			// Superseded: what this measured is about a network that has been
			// replaced, and publishing it would put another network's links
			// on the map.
			return
		}
		links := s.links()
		if ctx.Err() != nil {
			return
		}
		s.warmMu.Lock()
		s.warmed = true
		s.warmMu.Unlock()
		// What was measured survives the process, keyed by the geometry it
		// is about. On its own goroutine already, and after the staleness
		// checks, so what lands on disk is a matrix somebody saw.
		saveMatrix(s.matrixDir(), s.geomFP, s.eng.LinkCacheSnapshot())
		_, _ = st.Do(context.Background(), "links.set", links)
		_, _ = st.Do(context.Background(), "job.progress", state.Job{
			ID: "links", What: "measuring every link",
			Done: total, Total: total, Finished: true})
	}()
}

// rebuild starts the same network again from the world's seed.
//
// The engine is remade rather than rewound: an engine carries queued packets,
// per-node radio state and the firmware processes' own memory, and there is no
// honest way to unwind those to zero. Firmware is left alone, because
// restarting several hundred processes to change a seed is a different and
// much slower operation than the caller asked for.
func (s *Sim) rebuild(w *state.World) error {
	if len(s.nodes) == 0 {
		return fmt.Errorf("no network loaded")
	}
	seed := w.Seed
	if seed == 0 {
		seed = defaultSeed
	}
	s.buildSeeded(s.nodes, s.freqMHz, seed)
	w.NowMs, w.Seed = 0, seed
	w.Events, w.EventTotal = nil, 0
	return nil
}

// warming reports whether the link matrix is still being measured, which is
// the one thing a run must not start in front of.
func (s *Sim) warming() bool {
	s.warmMu.Lock()
	defer s.warmMu.Unlock()
	return s.warmCancel != nil && !s.warmed
}

// geometryFingerprint hashes everything a path loss depends on.
func geometryFingerprint(nodes []scenario.Node, freqMHz, excess float64) uint64 {
	h := fnv.New64a()
	b := make([]byte, 8)
	put := func(f float64) {
		binary.LittleEndian.PutUint64(b, math.Float64bits(f))
		_, _ = h.Write(b)
	}
	put(freqMHz)
	put(excess)
	for _, n := range nodes {
		_, _ = h.Write([]byte(n.Name))
		put(n.Position.Lat)
		put(n.Position.Lon)
		put(n.HeightAGLm)
		put(n.TxPowerDBm)
		// Noise figure is in here because the engine's path-loss cull decides
		// against the noise floor, so a node that got quieter can bring a pair
		// back that a previous run discarded. Leaving it out meant a stale
		// matrix loaded from disk and looked authoritative.
		put(n.NoiseFigureDB)
	}
	return h.Sum64()
}
