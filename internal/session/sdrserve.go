// Serving a placed observer's antenna to real SDR software.
//
// The workflow the plan asks for: place an observer on the map, start it,
// point SDR++ at the address, watch the simulated spectrum. The IQ comes
// from engine.ObserveSpan - the same shared synthesis the verdicts render
// from - and never from packet events, which is the whole point.
package session

import (
	"fmt"
	"sync"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/sdr"
)

// engineSource streams a receiver's IQ in simulated time: each pull renders
// the next span after the last, chasing the engine's clock. When the
// simulation pauses, the source idles at the pause point and serves the
// receiver's own noise floor - frozen time cannot honestly produce signal.
type engineSource struct {
	s   *Sim
	idx int
	// rate is fixed at attach: the receiver's bandwidth, one sample per Hz.
	rate float64

	mu     sync.Mutex
	atMs   float64
	primed bool
}

func (g *engineSource) SampleRateHz() float64 { return g.rate }

func (g *engineSource) NextSamples(n int) []complex128 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.s.eng == nil {
		return make([]complex128, n)
	}
	now := float64(g.s.eng.NowMs())
	if !g.primed {
		g.atMs, g.primed = now, true
	}
	spanMs := float64(n) / (g.rate / 1000)
	// Chase the clock without outrunning it; a paused engine holds the
	// stream at the pause point rather than inventing future air.
	if g.atMs+spanMs > now {
		g.atMs = now - spanMs
		if g.atMs < 0 {
			g.atMs = 0
		}
	}
	out := g.s.eng.ObserveSpan(g.idx, uint32(g.atMs), n)
	g.atMs += spanMs
	return out
}

func registerSDRServe(st *state.Store, s *Sim) {
	// sdr.serve: expose one node's antenna as an rtl_tcp source.
	st.Handle("sdr.serve", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		name, _ := stringField(p, "node")
		idx := -1
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				idx = i
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("no node named %q", name)
		}
		if s.sdrServers == nil {
			s.sdrServers = map[string]*sdr.RTLTCP{}
		}
		if old, ok := s.sdrServers[name]; ok {
			_ = old.Close()
			delete(s.sdrServers, name)
		}
		en, ok := s.eng.NodeByName(name)
		if !ok {
			return nil, fmt.Errorf("%s is not in the engine", name)
		}
		rate := en.Spec.Radio.BandwidthHz
		if rate <= 0 {
			rate = 250e3
		}
		srv, err := sdr.ServeRTLTCP("127.0.0.1:0",
			&engineSource{s: s, idx: idx, rate: rate})
		if err != nil {
			return nil, err
		}
		s.sdrServers[name] = srv
		w.Say(fmt.Sprintf("%s is an rtl_tcp source at %s - set the client's "+
			"sample rate to %.0f Hz", name, srv.Addr(), rate))
		return map[string]any{"node": name, "addr": srv.Addr(), "rate_hz": rate}, nil
	})

	st.Handle("sdr.stop", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		srv, ok := s.sdrServers[name]
		if !ok {
			return nil, fmt.Errorf("%s is not being served", name)
		}
		_ = srv.Close()
		delete(s.sdrServers, name)
		w.Say("stopped serving " + name)
		return map[string]any{"stopped": name}, nil
	})
}
