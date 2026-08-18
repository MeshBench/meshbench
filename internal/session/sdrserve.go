// Serving a placed observer's antenna to real SDR software.
//
// The workflow the plan asks for: place an observer on the map, start it,
// point SDR++ at the address, watch the simulated spectrum. The IQ comes
// from engine.ObserveSpan - the same shared synthesis the verdicts render
// from - and never from packet events, which is the whole point.
package session

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

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
	// pauseTick keys the noise served while the simulation is not advancing,
	// so a paused stream is fresh noise rather than one block repeated -
	// repeated IQ is exactly the striped garbage a client draws.
	pauseTick uint64
	// lastNow and stalled remember a fallback: while the clock has not moved
	// since, the next window is noise immediately rather than after another
	// full wait - a paused stream must still flow at rate.
	lastNow float64
	stalled bool
}

// observerLagMs is how far behind the simulation the stream deliberately
// runs. The engine advances in step-sized quanta on a scheduler with moods;
// serving right at the edge of simulated time turned every hiccup into a
// starved client and a streaked waterfall. A quarter second of cushion
// absorbs the jitter, and nobody watches a waterfall for currency.
const observerLagMs = 250

func (g *engineSource) SampleRateHz() float64 { return g.rate }

func (g *engineSource) NextSamples(n int) []complex128 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.s.eng == nil {
		return make([]complex128, n)
	}
	spanMs := float64(n) / (g.rate / 1000)
	if !g.primed {
		g.atMs = math.Max(0, float64(g.s.eng.NowMs())-observerLagMs-spanMs)
		g.primed = true
	}
	// The stream position only ever moves forward. Rewinding to chase the
	// clock re-served overlapping windows - repeated IQ - which is what a
	// waveform judgement running slower than the wall turned into striped
	// garbage on every transmission.
	//
	// Not yet simulated: wait for the engine, patiently - the lag cushion
	// means landing here at all is already the unusual case. A clock that
	// stopped moving is a pause: serve fresh noise at rate, immediately on
	// every window after the first, until it moves again.
	for wait := 0; g.atMs+spanMs > float64(g.s.eng.NowMs()); wait++ {
		now := float64(g.s.eng.NowMs())
		if (g.stalled && now == g.lastNow) || wait >= 120 {
			g.pauseTick++
			g.stalled, g.lastNow = true, now
			return g.s.eng.ObserveNoise(g.idx, g.pauseTick, n)
		}
		g.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		g.mu.Lock()
		if g.s.eng == nil {
			return make([]complex128, n)
		}
	}
	g.stalled = false
	// Far behind a fast simulation: jump back to the cushion rather than
	// stream minutes late. A real dongle drops samples on overflow too.
	if now := float64(g.s.eng.NowMs()); g.atMs+spanMs < now-observerLagMs-2000 {
		g.atMs = now - observerLagMs - spanMs
	}
	out := g.s.eng.ObserveSpan(g.idx, uint32(g.atMs), n)
	g.atMs += spanMs
	return out
}

// sdrServer is one serving observer and the rate its source was fixed at.
type sdrServer struct {
	srv    *sdr.RTLTCP
	rateHz float64
}

// sdrSources is what is currently served, for the observer windows: address,
// rate, and whether a client is on the line right now.
func (s *Sim) sdrSources() []state.SDRSource {
	out := make([]state.SDRSource, 0, len(s.sdrServers))
	for name, e := range s.sdrServers {
		out = append(out, state.SDRSource{
			Node: name, Addr: e.srv.Addr(), RateHz: e.rateHz,
			Attached: e.srv.Attached(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
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
			s.sdrServers = map[string]*sdrServer{}
		}
		if old, ok := s.sdrServers[name]; ok {
			_ = old.srv.Close()
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
		s.sdrServers[name] = &sdrServer{srv: srv, rateHz: rate}
		w.SDRSources = s.sdrSources()
		w.Say(fmt.Sprintf("%s is an rtl_tcp source at %s - the stream follows "+
			"the client's own rate setting (native %.0f Hz)", name, srv.Addr(), rate))
		return map[string]any{"node": name, "addr": srv.Addr(), "rate_hz": rate}, nil
	})

	st.Handle("sdr.stop", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		e, ok := s.sdrServers[name]
		if !ok {
			return nil, fmt.Errorf("%s is not being served", name)
		}
		_ = e.srv.Close()
		delete(s.sdrServers, name)
		w.SDRSources = s.sdrSources()
		w.Say("stopped serving " + name)
		return map[string]any{"stopped": name}, nil
	})
}
