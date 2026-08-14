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

	"github.com/MeshBench/meshbench/internal/gui/state"
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
	if s.eng == nil {
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

	nodes := s.eng.Nodes()
	out := make([]state.NodeStat, 0, len(nodes))
	live := map[int]bool{}
	for i, n := range nodes {
		st := state.NodeStat{Name: n.Spec.Name, Sent: n.Sent, Heard: n.Heard}
		if s.states != nil {
			st.State = s.states[n.Spec.Name]
		}
		if i < len(s.nodes) {
			st.Firmware = s.nodes[i].Firmware.Version
		}
		if n.Firmware != nil {
			st.Running = true
			st.Backend = n.Firmware.Backend.Kind()
			r := n.Firmware.Bridge.Stats()
			st.IRQReads, st.BusyReads = r.IRQReads, r.BusyReads
			st.BusyMs, st.Spurious = r.BusyMs, r.SpuriousUp
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
		return fmt.Errorf("no simulation")
	}
	n, ok := s.eng.NodeByName(name)
	if !ok {
		return fmt.Errorf("no node named %q", name)
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
		return fmt.Errorf("no simulation")
	}
	n, ok := s.eng.NodeByName(name)
	if !ok {
		return fmt.Errorf("no node named %q", name)
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
func (s *Sim) setFirmware(name, version string) error {
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			s.nodes[i].Firmware.Version = version
			// The engine keeps its own copy; see Engine.PinFirmware.
			if s.eng != nil {
				s.eng.PinFirmware(name, version)
			}
			return nil
		}
	}
	return fmt.Errorf("no node named %q", name)
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
	if s.states == nil {
		s.states = map[string]string{}
	}
	if what == "" {
		delete(s.states, name)
		return
	}
	s.states[name] = what
}

// Reflash stops a node, gives it a different build, and starts it again.
//
// The whole cycle, because firmware is chosen when a node launches: setting the
// version on a running node changes nothing until something else restarts it,
// and a control that appears to do nothing is one somebody presses twice.
func (s *Sim) Reflash(ctx context.Context, st *state.Store, name, version string, seed uint64) {
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
		if err := s.setFirmware(name, version); err != nil {
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
		_, _ = st.Do(ctx, "node.reflashed", name+" now runs "+version)
	}()
}
