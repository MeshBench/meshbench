// What each node is costing and doing.
//
// Two sources, and they answer different questions. The bridge's counters say
// what the firmware asked its radio - including how often the chip claimed the
// air was busy, which is the only way to tell a genuinely busy mesh from a
// radio that cries busy too readily. /proc says what the process costs, which
// is the question somebody asks when 154 of them are running and the machine
// has started swapping.
package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// cpuSampler turns cumulative CPU ticks into a share of one core.
//
// A rate needs two readings. Keeping the last one here rather than recomputing
// from process start means the number reflects what a node is doing now, not
// its average since boot - which for a node that was busy during a flood and
// is now idle are very different claims.
type cpuSampler struct {
	mu   sync.Mutex
	last map[int]cpuSample
}

type cpuSample struct {
	ticks uint64
	at    time.Time
}

func newCPUSampler() *cpuSampler { return &cpuSampler{last: map[int]cpuSample{}} }

// clockTicks is what /proc counts in. 100 on every Linux this runs on; read
// from the kernel would need cgo, and being wrong here scales every number by
// the same constant rather than changing which node is the expensive one.
const clockTicks = 100

func (c *cpuSampler) sample(pid int) (rssBytes int64, cpuPct float64, cpuMs int64) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, 0
	}
	// The process name can contain spaces and brackets, so fields are counted
	// from the closing bracket rather than from the start.
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return 0, 0, 0
	}
	f := strings.Fields(string(b)[i+1:])
	// After the name: state is f[0], so utime is field 14 overall = f[11].
	if len(f) < 22 {
		return 0, 0, 0
	}
	utime, _ := strconv.ParseUint(f[11], 10, 64)
	stime, _ := strconv.ParseUint(f[12], 10, 64)
	rssPages, _ := strconv.ParseInt(f[21], 10, 64)
	rssBytes = rssPages * int64(os.Getpagesize())

	now := time.Now()
	ticks := utime + stime
	c.mu.Lock()
	prev, seen := c.last[pid]
	c.last[pid] = cpuSample{ticks: ticks, at: now}
	c.mu.Unlock()
	cpuMs = int64(ticks) * 1000 / clockTicks
	if !seen {
		return rssBytes, 0, cpuMs
	}
	dt := now.Sub(prev.at).Seconds()
	if dt <= 0 || ticks < prev.ticks {
		return rssBytes, 0, cpuMs
	}
	return rssBytes, float64(ticks-prev.ticks) / clockTicks / dt * 100, cpuMs
}

// forget drops a process that has gone, so a long session does not accumulate
// a sample per node it has ever started.
func (c *cpuSampler) forget(live map[int]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for pid := range c.last {
		if !live[pid] {
			delete(c.last, pid)
		}
	}
}

// nodeStats collects a row per node.
//
// events is the tail the store already keeps, so last-sent and last-heard cost
// one pass over it rather than a second log kept per node - and they cannot
// disagree with the events table, which is the more useful property.
func (s *Sim) nodeStats(events []state.Event) []state.NodeStat {
	if s.liveEngine() == nil {
		return nil
	}
	if s.cpu == nil {
		s.cpu = newCPUSampler()
	}
	// Newest wins, so walk forwards and overwrite.
	type last struct {
		atMs uint32
		who  string
	}
	sent, heard := map[string]last{}, map[string]last{}
	for i := range events {
		e := &events[i]
		switch e.Kind {
		case "tx":
			sent[e.From] = last{atMs: e.AtMs, who: "flood"}
		case "rx":
			heard[e.To] = last{atMs: e.AtMs, who: e.From}
			sent[e.From] = last{atMs: e.AtMs, who: e.To}
		}
	}

	eng := s.liveEngine()
	nodes := eng.Nodes()
	// The counters come off the scoreboard rather than off the nodes, because
	// the scoreboard reads them under the engine's lock and this runs while a
	// run is stepping.
	counted := map[string]engine.Score{}
	for _, sc := range eng.Scoreboard() {
		counted[sc.Name] = sc
	}
	out := make([]state.NodeStat, 0, len(nodes))
	live := map[int]bool{}
	for _, n := range nodes {
		sc := counted[n.Spec().Name]
		st := state.NodeStat{Name: n.Spec().Name, Sent: sc.Sent, Heard: sc.Heard}
		st.State = s.stateOf(n.Spec().Name)
		// By name, not by position. The engine keeps its own list and there
		// is nothing holding the two in the same order, so indexing one with
		// the other's subscript reads some other node's build - and a board
		// read off the wrong node is a Hardware tab that appears on a node
		// without one and is missing from the node that has it.
		if spec, ok := s.nodeByName(n.Spec().Name); ok {
			st.Firmware = spec.Firmware.Version
			st.Board = spec.Firmware.Board
		}
		if n.Firmware != nil {
			st.Running = true
			st.Backend = n.Firmware.Backend.Kind()
			r := n.Firmware.Bridge.Stats()
			st.IRQReads, st.BusyReads = r.IRQReads, r.BusyReads
			st.BusyMs, st.Spurious = r.BusyMs, r.SpuriousUp
			// The board's own screen, where it has one and has drawn. A board
			// with no display and one that has drawn nothing both leave this
			// nil - which of the two it is comes from the board's declaration,
			// not from here.
			if scr, ok := n.Firmware.Backend.(interface {
				Screen() (int, int, int, bool, []byte, bool)
			}); ok {
				if w, h, bpp, on, bits, have := scr.Screen(); have {
					st.Screen = &state.Screen{
						Width: w, Height: h, BPP: bpp, On: on, Bits: bits}
				}
			}
			st.Radio = state.RadioState{
				Reported: r.Configured, GainReg: r.RxGainReg,
				Boosted: r.RxBoosted(), TxPowerDBm: r.TxPowerDBm,
				FemLive: r.FemEnabled, FemAtTx: uint8(r.FemAtTx),
				Mode: r.Mode, SF: r.SF, CR: r.CR,
				FreqHz: r.FreqHz, BandwidthHz: r.BandwidthHz,
				PreambleSyms: r.PreambleSyms,
				IRQMask:      r.IRQMask, IRQFlags: r.IRQFlags,
			}
			if p, ok := n.Firmware.Backend.(interface{ PID() int }); ok {
				st.PID = p.PID()
			}
		}
		if l, ok := sent[st.Name]; ok {
			st.LastSentMs, st.LastSentTo = l.atMs, l.who
		}
		if l, ok := heard[st.Name]; ok {
			st.LastHeardMs, st.LastHeardFrom = l.atMs, l.who
		}
		if st.PID > 0 {
			live[st.PID] = true
			st.RSSBytes, st.CPUPct, st.CPUms = s.cpu.sample(st.PID)
		}
		out = append(out, st)
	}
	s.cpu.forget(live)
	return out
}

// stopNode shuts one node's firmware down, leaving the node in the scenario.
//
// Stopping is how a node is told to go away, not killed: dropping the bridge
// ends its read loop, it reports its final counters and exits. Those counters
// are usually the only evidence about a node that was misbehaving.
func (s *Sim) stopNode(name string) error {
	if s.eng == nil {
		return ErrNoSimulation
	}
	n, ok := s.eng.NodeByName(name)
	if !ok {
		return noSuchNode(name)
	}
	if n.Firmware == nil {
		return fmt.Errorf("%s is not running firmware", name)
	}
	err := n.Firmware.Close()
	n.Firmware = nil
	return err
}

// startNode brings one node's firmware up again.
func (s *Sim) startNode(ctx context.Context, name string, seed uint64) error {
	if s.eng == nil {
		return ErrNoSimulation
	}
	n, ok := s.eng.NodeByName(name)
	if !ok {
		return noSuchNode(name)
	}
	if n.Firmware != nil {
		return fmt.Errorf("%s is already running", name)
	}
	// The whole-mesh attach, which skips every node that is already running.
	// Reusing it means one start path rather than two, and the one that gets
	// reused is the one with the resolution, concurrency and error handling
	// already in it.
	return s.eng.AttachNativeProgress(ctx, seed, nil)
}

// setFirmware changes which build a node will run next time it starts.
// Build is one firmware choice: a version, and where it came from.
//
// Board and Role travel with the version because a board image only means
// anything alongside them - "wadamesh" is not a build until it is wadamesh for
// a LilyGo_TDeck. Both empty is a host build, which is what every node had
// before board images could be chosen at all.
type Build struct {
	Version string
	Board   string
	Role    string
}

// Describe names a build the way somebody would say it aloud.
func (b Build) Describe() string {
	if b.Board == "" {
		return b.Version
	}
	return b.Version + " for " + b.Board
}

// refreshNodeBuild copies what a node is set to run back into the list the
// interface draws from.
//
// The two are separate on purpose - the scenario is what runs, the list is
// what is shown - but nothing was carrying a change from one to the other, so
// picking a build updated the table (which reads the live stats) and left the
// node's own window showing whatever it had been loaded with. Two views of one
// node disagreeing is worse than either being wrong.
// nodeByName is the session's copy of one node's specification.
func (s *Sim) nodeByName(name string) (scenario.Node, bool) {
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			return s.nodes[i], true
		}
	}
	return scenario.Node{}, false
}

func (s *Sim) refreshNodeBuild(w *state.World, name string) {
	for i := range s.nodes {
		if s.nodes[i].Name != name {
			continue
		}
		for j := range w.Nodes {
			if w.Nodes[j].Name == name {
				w.Nodes[j].Firmware = s.nodes[i].Firmware.Version
				w.Nodes[j].Board = s.nodes[i].Firmware.Board
			}
		}
		return
	}
}

func (s *Sim) setFirmware(name string, b Build) error {
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			s.nodes[i].Firmware.Version = b.Version
			// Cleared rather than left alone when a host build is chosen: a
			// node that keeps a board from its last image would try to run the
			// new one under an emulator it was never built for.
			s.nodes[i].Firmware.Board = b.Board
			if b.Role != "" {
				s.nodes[i].Firmware.Role = scenario.Role(b.Role)
			}
			// The engine keeps its own copy; see Engine.PinFirmware.
			if s.eng != nil {
				s.eng.PinFirmware(name, b.Version)
				s.eng.PinBoard(name, b.Board, scenario.Role(b.Role))
			}
			return nil
		}
	}
	return noSuchNode(name)
}

// history is a bounded ring of samples per node, for the graphs.
//
// Kept in the session rather than the panel because a panel that owns its own
// history loses it when the panel is closed, popped into a window, or drawn by
// a second front end - and the first question after "which node" is usually
// "was it always like that".
const historyLen = 240

type nodeHistory struct {
	mu   sync.Mutex
	rss  map[string][]int64
	cpu  map[string][]float64
	sent map[string][]int
}

func newNodeHistory() *nodeHistory {
	return &nodeHistory{
		rss:  map[string][]int64{},
		cpu:  map[string][]float64{},
		sent: map[string][]int{},
	}
}

// record appends this sample and drops the oldest.
func (h *nodeHistory) record(stats []state.NodeStat) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range stats {
		h.rss[s.Name] = pushInt64(h.rss[s.Name], s.RSSBytes)
		h.cpu[s.Name] = pushFloat(h.cpu[s.Name], s.CPUPct)
		h.sent[s.Name] = pushInt(h.sent[s.Name], s.Sent)
	}
}

// seriesFor is what the graphs draw.
func (h *nodeHistory) seriesFor(name string) state.NodeSeries {
	h.mu.Lock()
	defer h.mu.Unlock()
	return state.NodeSeries{
		Name: name,
		RSS:  append([]int64(nil), h.rss[name]...),
		CPU:  append([]float64(nil), h.cpu[name]...),
		Sent: append([]int(nil), h.sent[name]...),
	}
}

func pushInt64(s []int64, v int64) []int64 {
	s = append(s, v)
	if len(s) > historyLen {
		s = s[len(s)-historyLen:]
	}
	return s
}

func pushFloat(s []float64, v float64) []float64 {
	s = append(s, v)
	if len(s) > historyLen {
		s = s[len(s)-historyLen:]
	}
	return s
}

func pushInt(s []int, v int) []int {
	s = append(s, v)
	if len(s) > historyLen {
		s = s[len(s)-historyLen:]
	}
	return s
}

// setState records what a node is doing between the states it rests in.
//
// Kept here rather than inferred from whether a process exists, because the
// interesting moments are the ones where it does not: a row that goes blank
// while its firmware is being changed looks exactly like a node that has died.
func (s *Sim) setState(name, what string) {
	// Locked, because Reflash writes this from a goroutine of its own while
	// the store goroutine reads it to answer nodes.stats. That is a concurrent
	// map read and write, which Go does not merely tolerate badly - it kills
	// the process, taking the whole workbench and every emulated node with it.
	// Reflashing two nodes and then asking what is running was enough.
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	if s.states == nil {
		s.states = map[string]string{}
	}
	if what == "" {
		delete(s.states, name)
		return
	}
	s.states[name] = what
}

// stateOf reads what a node is doing between the states it rests in.
func (s *Sim) stateOf(name string) string {
	s.statesMu.Lock()
	defer s.statesMu.Unlock()
	return s.states[name]
}

// Reflash stops a node, gives it a different build, and starts it again.
//
// The whole cycle, because firmware is chosen when a node launches: setting the
// version on a running node changes nothing until something else restarts it,
// and a control that appears to do nothing is one somebody presses twice.
func (s *Sim) Reflash(ctx context.Context, st *state.Store, name string, b Build, seed uint64) {
	go func() {
		announce := func(what string) {
			s.setState(name, what)
			_, _ = st.Do(ctx, "nodes.stats", nil)
		}
		announce("stopping")
		if err := s.stopNode(name); err != nil {
			// Not running is not a failure here: the point is to end up
			// running the requested build.
			_ = err
		}
		announce("provisioning")
		if err := s.setFirmware(name, b); err != nil {
			s.setState(name, "")
			_, _ = st.Do(ctx, "node.reflash_failed", err.Error())
			return
		}
		announce("starting")
		if err := s.startNode(ctx, name, seed); err != nil {
			s.setState(name, "")
			_, _ = st.Do(ctx, "node.reflash_failed", err.Error())
			return
		}
		s.setState(name, "")
		_, _ = st.Do(ctx, "node.reflashed", name+" now runs "+b.Describe())
	}()
}
