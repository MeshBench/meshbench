// Frame timing, opt-in.
//
// The pre-P0 spike measured the map at 24 fps naive and 35 fps batched, and
// named viewport culling and path caching as the two remaining levers. Both
// are now in the map. A number measured on a spike cannot be closed by a
// change made somewhere else, so this measures the real component in the real
// binary.
//
// Two numbers, because they answer different questions. Delivered frames per
// second is what somebody sees, and it is capped by the display and shaped by
// the compositor. Cost per frame is the time this process spends building and
// submitting one, and that is the number the 16.7 ms budget is about: it is
// ours to fix, where the other is not.
package main

import (
	"fmt"
	"os"
	"sort"
	"time"
)

type fpsMeter struct {
	label  string
	frames int
	costs  []time.Duration
	worst  time.Duration
	last   time.Time
	since  time.Time
	log    *os.File
	// warm is how long to ignore before reporting, so the first tile decode
	// and the first shaping of every label do not land in the steady state.
	warm  time.Duration
	start time.Time
}

func newFPSMeter(label, path string) *fpsMeter {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fps log:", err)
	}
	now := time.Now()
	return &fpsMeter{label: label, log: f, warm: 3 * time.Second,
		start: now, since: now, last: now}
}

// frame is called once per delivered frame, with what that frame cost to
// build and submit.
func (m *fpsMeter) frame(cost time.Duration) {
	now := time.Now()
	if d := now.Sub(m.last); d > m.worst {
		m.worst = d
	}
	m.last = now
	m.frames++
	m.costs = append(m.costs, cost)
	if now.Sub(m.since) < time.Second {
		return
	}
	fps := float64(m.frames) / now.Sub(m.since).Seconds()
	line := fmt.Sprintf("%s  %5.1f fps delivered   cost/frame p50 %5.2f ms  p95 %5.2f ms  max %5.2f ms%s",
		m.label, fps, ms(m.pct(50)), ms(m.pct(95)), ms(m.pct(100)), m.warmNote(now))
	fmt.Fprintln(os.Stderr, line)
	if m.log != nil {
		fmt.Fprintln(m.log, line)
	}
	m.frames, m.worst, m.since, m.costs = 0, 0, now, m.costs[:0]
}

// pct is the nth percentile of this second's frame costs.
func (m *fpsMeter) pct(n int) time.Duration {
	if len(m.costs) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), m.costs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := len(s) * n / 100
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func (m *fpsMeter) warmNote(now time.Time) string {
	if now.Sub(m.start) < m.warm {
		return "   (warming up, ignore)"
	}
	return ""
}
