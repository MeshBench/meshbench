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

	"github.com/A13xB0/meshcoresim/internal/gui/state"
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

func (c *cpuSampler) sample(pid int) (rssBytes int64, cpuPct float64) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0
	}
	// The process name can contain spaces and brackets, so fields are counted
	// from the closing bracket rather than from the start.
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return 0, 0
	}
	f := strings.Fields(string(b)[i+1:])
	// After the name: state is f[0], so utime is field 14 overall = f[11].
	if len(f) < 22 {
		return 0, 0
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
	if !seen {
		return rssBytes, 0
	}
	dt := now.Sub(prev.at).Seconds()
	if dt <= 0 || ticks < prev.ticks {
		return rssBytes, 0
	}
	return rssBytes, float64(ticks-prev.ticks) / clockTicks / dt * 100
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
func (s *Sim) nodeStats() []state.NodeStat {
	if s.eng == nil {
		return nil
	}
	if s.cpu == nil {
		s.cpu = newCPUSampler()
	}
	nodes := s.eng.Nodes()
	out := make([]state.NodeStat, 0, len(nodes))
	live := map[int]bool{}
	for i, n := range nodes {
		st := state.NodeStat{Name: n.Spec.Name, Sent: n.Sent, Heard: n.Heard}
		if i < len(s.nodes) {
			st.Firmware = s.nodes[i].Firmware.Version
		}
		if n.Firmware != nil {
			st.Running = true
			st.Backend = n.Firmware.Backend.Kind()
			r := n.Firmware.Bridge.Stats()
			st.IRQReads, st.BusyReads = r.IRQReads, r.BusyReads
			st.BusyMs, st.Spurious = r.BusyMs, r.SpuriousUp
			if p, ok := n.Firmware.Backend.(interface{ PID() int }); ok {
				st.PID = p.PID()
			}
		}
		if st.PID > 0 {
			live[st.PID] = true
			st.RSSBytes, st.CPUPct = s.cpu.sample(st.PID)
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
			return nil
		}
	}
	return fmt.Errorf("no node named %q", name)
}
