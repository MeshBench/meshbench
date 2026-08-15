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
	"github.com/MeshBench/meshbench/internal/terrain"
)

// Sim holds the engine and the scenario it was built from.
type Sim struct {
	// ui is whatever is drawing this session, if anything.
	ui UI
	// consoles is one scrollback per node, keyed by name.
	consoles map[string]*console.Buf
	// cold reports that the engine has been rebuilt and its link cache is
	// empty, so the next thing that can warm it should.
	cold bool
	// warmed reports that the matrix has been measured for the engine as it
	// stands. Cleared by a rebuild, set when a warm finishes uncancelled.
	warmed bool
	// gpuWarm is whether the link matrix is measured on the GPU when one can
	// answer to the same accuracy. Off by default: it reads a rasterised
	// height grid rather than the DEM, which is the same answer on a county
	// and a different one on a continent.
	gpuWarm bool
	gpuMu   sync.Mutex
	// gpuProbe is what asking this machine for a GPU answered, kept because
	// asking twice opens a device twice.
	gpuProbe *gpuProbe
	// gpuAsked records that the machine has been asked whether it has a GPU,
	// so the answer is not re-opened on every warm.
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
	// fleetPending is a fleet command sent and not yet answered, held until the
	// engine has run far enough for the nodes to have replied.
	fleetPending *fleetPending
	// publishedNet is what the firmware catalogue offers, fetched once; nil
	// until the fetch has answered, empty after a fetch that failed.
	publishedNet      []publishedBuild
	fetchingPublished bool
	// imp is what has been fetched from a deployment but not yet applied.
	imp *importState
	// capturePath is where frames are being written, if anywhere.
	capturePath string
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
	// fixturePath is what project.open last loaded, so an experiment manifest
	// can name the network it was defined against rather than just its
	// geometry fingerprint - a fingerprint says two networks agree, not what
	// either one is called.
	fixturePath string
	// regRunning is a regression directory run in flight, so a second one
	// cannot start into the middle of the first.
	regRunning bool
	// boardProbing is a hardware capability probe in flight - one board at a
	// time, since each is a real emulator boot.
	boardProbing bool
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
			s.eng.RestoreLinkCache(carried)
			s.cold, s.warmed = false, true
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
	eng, warmNodes, freqMHz := s.eng, s.nodes, s.freqMHz
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

// trailsSince turns the engine's events into map trails.
//
// Only "tx" and "rx" - a "miss" is a reception that did not happen, and drawing
// it as traffic would put a line on the map for a packet that never arrived.
// A tx nobody received still gets a trail, with To of -1, because a repeater
// shouting into an empty valley is exactly the thing somebody is looking for.
func (s *Sim) trailsSince(fromMs uint32, index map[string]int) []state.Trail {
	if s.eng == nil {
		return nil
	}
	var out []state.Trail
	for _, e := range s.eng.EventsSince(fromMs) {
		from, ok := index[e.From]
		if !ok {
			continue
		}
		switch e.Kind {
		case "tx":
			out = append(out, state.Trail{From: from, To: -1, AtMs: e.AtMs})
		case "rx":
			to, ok := index[e.To]
			if !ok {
				continue
			}
			out = append(out, state.Trail{
				From: from, To: to, AtMs: e.AtMs, Delivered: true})
		}
	}
	return out
}

// eventTail is the most recent n events, oldest first, and the total.
func (s *Sim) eventTail(n int) ([]state.Event, int) {
	if s.eng == nil {
		return nil, 0
	}
	all, total := s.eng.EventsTail(n)
	out := make([]state.Event, 0, len(all))
	for _, e := range all {
		out = append(out, state.Event{
			AtMs: e.AtMs, Kind: e.Kind, From: e.From, To: e.To,
			MessageID: e.MessageID, PacketID: e.PacketID,
			SNRdB: e.SNRdB, Detail: e.Detail,
			Class: engine.EventClass(e.Kind, e.Detail),
		})
	}
	return out, total
}

// eventCounts is the whole run's events by class, in the snapshot's shape.
func (s *Sim) eventCounts() state.EventCounts {
	if s.eng == nil {
		return state.EventCounts{}
	}
	c := s.eng.EventCounts()
	return state.EventCounts{
		Sent: c["sent"], Received: c["received"], HalfDuplex: c["half-duplex"],
		Interference: c["interference"], Floor: c["floor"],
	}
}

// scores is the engine's own scoreboard, projected.
func (s *Sim) scores() []state.Score {
	if s.eng == nil {
		return nil
	}
	sb := s.eng.Scoreboard()
	out := make([]state.Score, 0, len(sb))
	for _, v := range sb {
		out = append(out, state.Score{
			Name: v.Name, Sent: v.Sent, Heard: v.Heard,
			AirtimeMs: v.AirtimeMs, DutyCyclePct: v.DutyCyclePct,
			UniqueDelivery: v.UniqueDelivery, RedundantRelay: v.RedundantRelay,
			AirtimePayloadMs:   v.AirtimePayloadMs,
			AirtimeOverheadMs:  v.AirtimeOverheadMs,
			AirtimeRedundantMs: v.AirtimeRedundantMs,
		})
	}
	return out
}

// budgetsFor breaks down the strongest link at a node, both ways.
//
// The strongest rather than a chosen one, because the question a budget panel
// answers when nothing has been picked is "how is this node doing at all",
// and its best link is the honest answer to that.
func (s *Sim) budgetsFor(at int, links []state.Link) []state.Budget {
	if s.eng == nil || at < 0 || at >= len(s.nodes) {
		return nil
	}
	best, bestM := -1, math.Inf(-1)
	for _, l := range links {
		if !l.Known {
			continue
		}
		other := -1
		if l.A == at {
			other = l.B
		} else if l.B == at {
			other = l.A
		}
		if other < 0 || l.MarginDB <= bestM {
			continue
		}
		best, bestM = other, l.MarginDB
	}
	if best < 0 {
		return nil
	}
	loss, ok := s.eng.PathLossForTest(at, best)
	if !ok {
		return nil
	}
	a, b := s.nodes[at], s.nodes[best]
	return []state.Budget{
		{From: a.Name, To: b.Name, MarginDB: linkbudget.OneWayDB(a, b, loss),
			Terms: termsOf(linkbudget.Terms(a, b, loss))},
		{From: b.Name, To: a.Name, MarginDB: linkbudget.OneWayDB(b, a, loss),
			Terms: termsOf(linkbudget.Terms(b, a, loss))},
	}
}

func termsOf(in []linkbudget.Term) []state.BudgetTerm {
	out := make([]state.BudgetTerm, 0, len(in))
	for _, t := range in {
		out = append(out, state.BudgetTerm{Name: t.Name, DB: t.DB})
	}
	return out
}

// startFirmware brings real MeshCore up on every node that runs it.
//
// This is the thing the Gio workbench was missing entirely: it built an engine
// and never attached firmware, so nothing relayed and a packet had to be
// injected to make anything happen at all. A simulator that does not run the
// firmware is a channel model with a map on it.
func (s *Sim) startFirmware(st *state.Store, seed uint64) {
	if s.eng == nil || s.starting.Swap(true) {
		return
	}
	go func() {
		defer s.starting.Store(false)
		ctx := context.Background()
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "firmware", What: "starting firmware on every node"})
		err := s.eng.AttachNativeProgress(ctx, seed, func(done, total int) {
			// Every tenth node: a verb per node would make the queue the slow
			// part of starting 154 processes.
			if done%10 != 0 && done != total {
				return
			}
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "firmware", What: "starting firmware on every node",
				Done: done, Total: total})
		})
		if err != nil {
			_, _ = st.Do(ctx, "firmware.failed", err.Error())
			return
		}
		if n := s.provisionAll(); n > 0 {
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "firmware", What: "telling every node what it is",
				Done: n, Total: n})
			// Time has to move for the firmware to read what was queued at
			// its serial input; the old workbench steps sixty here too.
			_, _ = st.Do(ctx, "sim.settle", nil)
		}
		_, _ = st.Do(ctx, "firmware.started", nil)
	}()
}

// firmwareCount is how many nodes are running firmware right now.
func (s *Sim) firmwareCount() int {
	if s.eng == nil {
		return 0
	}
	return s.eng.FirmwareCount()
}

// Close shuts the simulation down, firmware included.
//
// Safe on a Sim that never built an engine, because the common shutdown path
// is a workbench closed before anything was loaded.
func (s *Sim) Close() {
	if s.eng == nil {
		return
	}
	for name, l := range s.served {
		_ = l.Close()
		delete(s.served, name)
	}
	_ = s.eng.Close()
	s.eng = nil
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

// provisionAll tells every node what it is, as soon as its firmware is up.
//
// This is the step that decides whether anything relays. A node that has not
// been told its regions holds none, so it forwards nothing and reports no
// error - which is what a mesh of three hundred nodes sitting in total silence
// for four minutes looked like. The old workbench does this in attachFirmware;
// this build did it only inside an experiment, so the one path an operator
// actually uses - press play - brought up MeshCore everywhere and told it
// nothing.
//
// The same lines the provisioning panel shows, so what is read and what is
// sent cannot drift apart. On the starting goroutine rather than in a verb,
// because three hundred nodes times seven commands is work proportional to the
// network and that does not belong on the store's thread.
func (s *Sim) provisionAll() int {
	if s.eng == nil {
		return 0
	}
	done := 0
	for _, n := range s.nodes {
		en, ok := s.eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		sent := false
		for _, line := range s.provisionLines(n) {
			if line.Comment {
				continue
			}
			if err := en.Firmware.Bridge.Type([]byte(line.Command + "\r\n")); err != nil {
				break
			}
			sent = true
		}
		if sent {
			done++
		}
	}
	return done
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
	}
	return h.Sum64()
}
