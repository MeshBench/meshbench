package engine

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/A13xB0/meshcoresim/internal/firmware"
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
// concurrency.
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
		if !n.Spec.Kind.RunsFirmware() || n.Firmware != nil {
			continue
		}
		todo++
		role := n.Spec.Firmware.Role
		if role == "" {
			role = n.Spec.Kind.Application()
		}
		key := role + "@" + n.Spec.Firmware.Version
		if _, ok := resolved[key]; ok {
			continue
		}
		path, err := firmware.Resolve(ctx, "", role, n.Spec.Firmware.Version, firmware.DefaultCacheDir())
		if err != nil {
			if resolveErr == nil {
				resolveErr = fmt.Errorf("%s: %w", n.Spec.Name, err)
			}
			continue
		}
		resolved[key] = path
	}

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
		role := n.Spec.Firmware.Role
		if role == "" {
			role = n.Spec.Kind.Application()
		}
		// One resolve per distinct build, not per node. Six hundred repeaters
		// share one binary; resolving each separately made six hundred GitHub
		// API calls, and the unauthenticated rate limit turned node 61 onward
		// into "403 Forbidden" — which read as missing firmware.
		key := role + "@" + n.Spec.Firmware.Version
		path, ok := resolved[key]
		if !ok {
			// Its build did not resolve above. One node's missing build must
			// not abandon the other two hundred.
			fail(fmt.Errorf("%s: no build for %s", n.Spec.Name, key))
			return
		}
		fw, err := firmware.Start(ctx, n.Spec.Name, &firmware.Native{
			Path:    path,
			Role:    role,
			WorkDir: firmware.NodeWorkDir(n.Spec.Name),
			Seed:    seed + uint64(i)*0x9E3779B97F4A7C15,
			SF:      e.Config.SF, BandwidthKHz: e.Config.BandwidthHz / 1000,
			CodingRate: e.Config.CodingRate,
		})
		if err != nil {
			fail(fmt.Errorf("start %s: %w", n.Spec.Name, err))
			return
		}
		// Start returns once the process exists, not once it has connected. The
		// engine ticks immediately afterwards, and a tick to a bridge with
		// nothing on the other end fails — so the wait is not a convenience,
		// it is the difference between working and not.
		if err := waitAttached(ctx, fw, attachTimeout); err != nil {
			_ = fw.Close()
			fail(fmt.Errorf("%s: %w", n.Spec.Name, err))
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
			fail(fmt.Errorf("%s: boot offset: %w", n.Spec.Name, err))
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
					progress(d, todo)
				}
			}
		}()
	}
	for i, n := range nodes {
		if !n.Spec.Kind.RunsFirmware() || n.Firmware != nil {
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

// attachTimeout is how long a firmware process gets to connect back.
const attachTimeout = 10 * time.Second

// bootAdvanceTimeout is how long a node gets to simulate its boot offset.
//
// Proportional, because the work is: a node told to advance two minutes runs
// two minutes of firmware, radio driver and virtual chip. The floor keeps a
// zero offset from having no patience at all.
func bootAdvanceTimeout(offsetMs uint32) time.Duration {
	d := time.Duration(offsetMs) * time.Millisecond * 2
	if d < attachTimeout {
		return attachTimeout
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

// FirmwareCount is how many nodes are running a real build.
func (e *Engine) FirmwareCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, node := range e.nodes {
		if node.Firmware != nil {
			n++
		}
	}
	return n
}

// NodeByName finds a node, for the console and the inspector.
func (e *Engine) NodeByName(name string) (*Node, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range e.nodes {
		if n.Spec.Name == name {
			return n, true
		}
	}
	return nil, false
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

// bootSpreadMs is the window boot offsets are drawn from: the repeater's own
// default advert interval, two minutes.
const bootSpreadMs = 120_000
