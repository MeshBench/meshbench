package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/native"
)

// AttachNative starts a real MeshCore build for every node that runs firmware.
//
// This is what separates the project from a packet-level simulator, and until
// something calls it the engine is only exercising its own channel. A node with
// firmware relays because MeshCore decided to, on an SNR that came out of a
// demodulator; a node without it relays because a test injected something.
//
// Nodes that do not run firmware — SDR observers, and custom emitters that are
// only present to interfere — are skipped rather than given one.
func (e *Engine) AttachNative(ctx context.Context, seed uint64) error {
	return e.AttachNativeProgress(ctx, seed, nil)
}

// AttachNativeProgress is AttachNative with a progress callback and bounded
// concurrency. The callback is serialised, so it may write to variables the
// caller reads after this returns without a lock of its own.
//
// Both exist for the same reason: a Scotland-sized scenario is 155 nodes, and
// starting them one at a time is minutes of work with nothing said about it.
// Driven from the control socket, which runs on the frame thread, that is a
// window frozen for the duration - indistinguishable from a crash, and reported
// as one.
//
// progress is called from the calling goroutine's perspective at each
// completion, with the count done and the total to do. It may be nil.
func (e *Engine) AttachNativeProgress(ctx context.Context, seed uint64, progress func(done, total int)) error {
	// One whole-mesh attach at a time. The second caller waits here, and by
	// the time it runs the first has set Firmware on every node it started, so
	// the filter below finds them running and leaves them alone rather than
	// booting a second emulator onto the same node's storage.
	e.attachMu.Lock()
	defer e.attachMu.Unlock()

	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	// Resolving is per distinct build and hits the network on a cache miss, so
	// it happens once, up front, rather than racing inside the workers.
	resolved := map[string]string{}
	var resolveErr error
	todo := 0
	for _, n := range nodes {
		if !n.Spec().Kind.RunsFirmware() || n.Firmware != nil {
			continue
		}
		todo++
		// An emulated node's firmware is a published board image, already in
		// the cache. Resolving a native build for it would fail on a version
		// that was never built for this machine, and take the whole attach down
		// with it.
		if n.Spec().Firmware.Emulated() {
			continue
		}
		role := n.Spec().Firmware.Role
		if role == "" {
			role = n.Spec().Kind.Application()
		}
		key := string(role) + "@" + n.Spec().Firmware.Version
		if _, ok := resolved[key]; ok {
			continue
		}
		path, err := firmware.Resolve(ctx, "", string(role), n.Spec().Firmware.Version,
			firmware.DefaultCacheDir())
		if err != nil {
			if resolveErr == nil {
				resolveErr = fmt.Errorf("%s: %w", n.Spec().Name, err)
			}
			continue
		}
		resolved[key] = path
	}
	e.mu.Lock()
	e.builds = e.builds[:0]
	for key, path := range resolved {
		e.builds = append(e.builds, Build{Key: key, Path: path, Sum: shortSum(path)})
	}
	sort.Slice(e.builds, func(i, j int) bool { return e.builds[i].Key < e.builds[j].Key })
	e.mu.Unlock()

	// Bounded concurrency: the work is process startup and a socket handshake,
	// so it is latency, not CPU, and running it one at a time wastes all of it.
	// Bounded rather than unbounded because each one is a real process.
	workers := runtime.NumCPU()
	if workers > 12 {
		workers = 12
	}
	if workers < 1 {
		workers = 1
	}

	var mu sync.Mutex
	// Progress callbacks are serialised on their own lock, not on mu.
	// Every worker reports, so an unguarded callback runs concurrently
	// with itself and races whatever the caller's closure writes - which
	// is a bug in every caller rather than in one of them, so it is fixed
	// here. Its own lock, because holding mu across somebody else's code
	// invites a deadlock the callers cannot see.
	var pmu sync.Mutex
	attached, failed, done := 0, 0, 0
	var firstErr = resolveErr

	type job struct {
		i int
		n *Node
	}
	jobs := make(chan job)
	var wg sync.WaitGroup

	fail := func(err error) {
		mu.Lock()
		failed++
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	// work attaches one node. The caller has already filtered to nodes that
	// need firmware, so there is nothing to skip here.
	work := func(i int, n *Node) {
		// A seed per node, derived from the run's seed and the node's index.
		// Sharing one seed would give every node the same identity, and a mesh
		// where every repeater has the same public key does not behave like a
		// mesh at all.
		// Which application this node runs. Not a property of the engine and
		// not a switch on a node type: the scenario says which firmware is
		// loaded, and that is the whole of what makes a node a repeater rather
		// than a companion.
		role := n.Spec().Firmware.Role
		if role == "" {
			role = n.Spec().Kind.Application()
		}
		// One resolve per distinct build, not per node. Six hundred repeaters
		// share one binary; resolving each separately made six hundred GitHub
		// API calls, and the unauthenticated rate limit turned node 61 onward
		// into "403 Forbidden" — which read as missing firmware.
		// Which backend runs this node is the node's own business: a board
		// means published hardware firmware under an emulator, and no board
		// means a build for this machine. Not a switch on node type, and not a
		// global mode - a scenario mixing the two is the point of having both.
		var backend firmware.Backend
		if n.Spec().Firmware.Emulated() {
			em, err := emulatedBackend(n.Spec(), e.Config.UnverifiedWiring)
			if err != nil {
				fail(fmt.Errorf("%s: %w", n.Spec().Name, err))
				return
			}
			backend = em
		} else {
			key := string(role) + "@" + n.Spec().Firmware.Version
			path, ok := resolved[key]
			if !ok {
				// Its build did not resolve above. One node's missing build
				// must not abandon the other two hundred.
				fail(fmt.Errorf("%s: no build for %s", n.Spec().Name, key))
				return
			}
			// The firmware's own diagnostics are the only window into a native
			// node, and discarding them meant a node that stopped ticking left
			// nothing behind to say why. An emulated node keeps console.log
			// beside it for the same reason; this is the native equivalent.
			dir := firmware.NodeWorkDir(n.Spec().Name)
			var stderr io.Writer
			if err := os.MkdirAll(dir, 0o755); err == nil {
				if f, err := os.Create(filepath.Join(dir, "stderr.log")); err == nil {
					stderr = f
				}
			}
			backend = &native.Native{
				Path:    path,
				Role:    string(role),
				WorkDir: dir,
				Log:     stderr,
				Seed:    seed + uint64(i)*0x9E3779B97F4A7C15,
				SF:      e.Config.SF, BandwidthKHz: e.Config.BandwidthHz / 1000,
				CodingRate: e.Config.CodingRate,
			}
		}
		fw, err := firmware.Start(ctx, n.Spec().Name, backend)
		if err != nil {
			fail(fmt.Errorf("start %s: %w", n.Spec().Name, err))
			return
		}
		// Start returns once the process exists, not once it has connected. The
		// engine ticks immediately afterwards, and a tick to a bridge with
		// nothing on the other end fails — so the wait is not a convenience,
		// it is the difference between working and not.
		if err := waitAttached(ctx, fw, attachBudget(workers)); err != nil {
			_ = fw.Close()
			fail(fmt.Errorf("%s: %w", n.Spec().Name, err))
			return
		}

		// A boot offset per node, so they do not all start their timers at the
		// same instant.
		//
		// Real repeaters are powered on weeks apart; every one of these starts
		// at t=0 with an identical timer phase, so their adverts fire on the
		// same millisecond for ever — a permanent, self-inflicted collision
		// that exists in no real network. Derived from the run seed, so it is
		// stagger rather than noise: the same scenario still replays exactly.
		//
		// Up to the advert interval itself (the repeater default is 2 minutes),
		// because a stagger smaller than the period being staggered only moves
		// the pile-up.
		off := uint32(0)
		if e.StaggerBoot {
			off = bootOffsetMs(seed, i)
		}
		// Bounded, like the attach before it. This wait is why a Scotland-sized
		// run froze the application outright: the node's process had gone, so
		// the ack it waits for could never arrive, and the background context
		// gave it no deadline to give up at.
		//
		// Scaled by how much simulated time is being asked for, not fixed. The
		// boot offset can be a couple of minutes, and since the radio became
		// real - MeshCore's driver over RadioLib over a virtual chip - each
		// simulated millisecond costs far more CPU than the old stub did. A
		// timeout sized for that stub failed every node in a 154-node scenario
		// on a stagger it had happily handled the week before.
		advCtx, cancel := context.WithTimeout(ctx, bootAdvanceTimeout(off))
		err = fw.Bridge.Advance(advCtx, off)
		cancel()
		if err != nil {
			_ = fw.Close()
			fail(fmt.Errorf("%s: boot offset: %w", n.Spec().Name, err))
			return
		}

		e.mu.Lock()
		e.nodes[i].Firmware = fw
		e.nodes[i].BootOffsetMs = off
		e.mu.Unlock()
		mu.Lock()
		attached++
		mu.Unlock()
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				work(j.i, j.n)
				mu.Lock()
				done++
				d := done
				mu.Unlock()
				if progress != nil {
					pmu.Lock()
					progress(d, todo)
					pmu.Unlock()
				}
			}
		}()
	}
	for i, n := range nodes {
		if !n.Spec().Kind.RunsFirmware() || n.Firmware != nil {
			continue
		}
		jobs <- job{i, n}
	}
	close(jobs)
	wg.Wait()
	if failed > 0 {
		// A partial mesh is a different network, not a slightly worse one, so
		// the caller hears exactly how partial. The nodes that did start stay
		// up: a fix (a downloaded build, a freed rate limit) can attach the
		// rest without restarting these.
		return fmt.Errorf("engine: firmware on %d of %d nodes; %d failed, first: %w",
			attached, attached+failed, failed, firstErr)
	}
	return nil
}

// attachBudget is how long a firmware process gets to connect back, scaled by
// how many are being brought up at once.
//
// The connect itself is a socket handshake - milliseconds when a node has a
// core to itself. But up to workers processes start together, and each one
// brings MeshCore's loop, RadioLib and a virtual chip up before it answers; on
// a busy machine the last of a full dozen is scheduled late enough to miss a
// flat ten seconds, which is a loaded runner rather than a hung node. So the
// budget grows with the contention, for the same reason bootAdvanceTimeout
// does. Generous on purpose: this deadline exists to turn a process that will
// never connect into an error, not to police startup latency.
func attachBudget(workers int) time.Duration {
	if d := time.Duration(workers) * 5 * time.Second; d > 30*time.Second {
		return d
	}
	return 30 * time.Second
}

// bootAdvanceTimeout is how long a node gets to simulate its boot offset.
//
// Proportional, because the work is: a node told to advance two minutes runs
// two minutes of firmware, radio driver and virtual chip. The floor keeps a
// zero offset from having no patience at all.
func bootAdvanceTimeout(offsetMs uint32) time.Duration {
	// Twenty times the simulated span, with a generous floor.
	//
	// The factor is not caution, it is measurement: a dozen nodes advance
	// concurrently, each running MeshCore's loop, its driver, RadioLib and a
	// virtual chip for every simulated millisecond, and together they run
	// several times slower than real time. A factor of two failed all 154 nodes
	// on a five-second offset.
	//
	// Generous on purpose. This deadline exists to turn a hang into an error,
	// not to police performance - if it ever fires, something is stuck.
	d := time.Duration(offsetMs) * time.Millisecond * 20
	if d < 60*time.Second {
		return 60 * time.Second
	}
	return d
}

func waitAttached(ctx context.Context, n *firmware.Node, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for !n.Bridge.Attached() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("firmware started but never connected within %v", timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// bootOffsetMs is how far into its own life a node already is when the run
// starts.
//
// Splitmix64 over the run seed and the node index: deterministic, cheap, and
// well distributed for adjacent indices — a plain multiply leaves the low bits
// correlated, which would stagger nodes into a handful of groups rather than
// spreading them.
func bootOffsetMs(seed uint64, i int) uint32 {
	x := seed + uint64(i+1)*0x9E3779B97F4A7C15
	x ^= x >> 30
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 27
	x *= 0x94D049BB133111EB
	x ^= x >> 31
	return uint32(x % bootSpreadMs)
}

// bootSpreadMs is the window boot offsets are drawn from.
//
// Two minutes - the repeater's own advert interval - was the right window when
// the radio was a stub: a node's boot offset costs nothing to simulate if
// nothing happens during it. With MeshCore's real driver over RadioLib over a
// virtual chip, every one of those milliseconds runs the firmware's loop and a
// pile of SPI, and 154 nodes drawing from a two-minute window took half an hour
// to attach.
//
// Eight seconds keeps what the stagger is for. The point was never the width of
// the window but that nodes do not all start their advert timers on the same
// millisecond, which eight seconds breaks just as thoroughly - and MeshCore
// jitters its adverts on top of this anyway.
const bootSpreadMs = 8_000
