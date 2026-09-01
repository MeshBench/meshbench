// Serving a placed observer's antenna to real SDR software.
//
// The workflow the plan asks for: place an observer on the map, start it,
// point SDR++ at the address, watch the simulated spectrum. The IQ comes
// from the same shared synthesis the verdicts render from - never from
// packet events, which is the whole point.
package sdr

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/sdr"
)

// observerLagMs is how far behind the simulation the stream deliberately
// runs. The engine advances in step-sized quanta on a scheduler with moods;
// rendering right at the edge of simulated time turned every hiccup into a
// starved client. Nobody watches a waterfall for currency.
const observerLagMs = 250

// pumpSpanMs is the producer's render quantum, and ringTargetMs how much
// finished stream it keeps ahead of the client. The ring is what lets a
// delivery burst after a scheduling hiccup be served instantly instead of
// blocking against the engine - the blocking is what oscillated: a stall
// grew the deficit, the deficit demanded future air, waiting for it grew
// the next stall, and the stream stayed choppy until a pause reset it.
const (
	pumpSpanMs   = 25
	ringTargetMs = 1500
)

// engineSource streams a receiver's signal-only IQ in simulated time. A
// producer goroutine renders ahead of the client as far as the engine's
// clock allows; the client-facing side only ever drains the ring, so the
// delivery path never calls into the engine at all. The front-end noise
// floor is the server's to add, at this source's stated density - which is
// what lets a paused simulation stream an honest bare floor: the producer
// parks and feeds the ring silence.
type engineSource struct {
	s   *session.Sim
	idx int
	// rate is fixed at attach: the receiver's bandwidth, one sample per Hz.
	rate float64
	// psd is the receiver's noise density, handed to the server so the
	// floor it paints and the floor the verdicts hear are the same claim.
	psd float64

	mu  sync.Mutex
	buf []complex128
	// atSample is the stream position on the receiver's own sample clock.
	// Milliseconds are not whole samples at every bandwidth, and walking
	// the stream in ms tore its phase at every span seam.
	atSample uint64
	primed   bool
	stop     chan struct{}
	closed   bool

	// lastNow and lastMove watch the engine's clock, so a stopped clock is
	// recognised as a pause rather than waited on forever.
	lastNow  float64
	lastMove time.Time
}

func newEngineSource(s *session.Sim, idx int, rate, psd float64) *engineSource {
	g := &engineSource{s: s, idx: idx, rate: rate, psd: psd,
		stop: make(chan struct{}), lastMove: time.Now()}
	go g.pump()
	return g
}

func (g *engineSource) SampleRateHz() float64 { return g.rate }
func (g *engineSource) NoisePSD() float64     { return g.psd }

func (g *engineSource) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed {
		g.closed = true
		close(g.stop)
	}
}

// pump renders the stream ahead of the client, one small span at a time.
func (g *engineSource) pump() {
	n := int(pumpSpanMs * g.rate / 1000)
	spms := g.rate / 1000 // samples per simulated millisecond
	spanMs := float64(n) / spms
	for {
		select {
		case <-g.stop:
			return
		default:
		}
		if g.s.Engine() == nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		g.mu.Lock()
		buffered := float64(len(g.buf)) / g.rate * 1000
		g.mu.Unlock()
		if buffered >= ringTargetMs {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		now := float64(g.s.Engine().NowMs())
		if now != g.lastNow {
			g.lastNow, g.lastMove = now, time.Now()
		}
		g.mu.Lock()
		if !g.primed {
			g.atSample = uint64(math.Max(0, now-observerLagMs-spanMs) * spms)
			g.primed = true
		}
		at := g.atSample
		g.mu.Unlock()
		atMs := float64(at) / spms
		switch {
		case atMs+spanMs <= now:
			// Far behind a fast simulation: jump back to the cushion rather
			// than stream minutes late; a real dongle drops on overflow too.
			if atMs+spanMs < now-observerLagMs-2000 {
				at = uint64((now - observerLagMs - spanMs) * spms)
			}
			out := g.s.Engine().ObserveSignalAt(g.idx, at, n)
			g.mu.Lock()
			g.buf = append(g.buf, out...)
			g.atSample = at + uint64(n)
			g.mu.Unlock()
		case time.Since(g.lastMove) > 400*time.Millisecond:
			// The clock has stopped: a pause. The stream stays alive on the
			// front-end floor alone - silence here, noise at the server -
			// and the position holds, so play resumes exactly where the air
			// left off.
			g.mu.Lock()
			g.buf = append(g.buf, make([]complex128, n)...)
			g.mu.Unlock()
		default:
			// Simulated time is close behind; give the engine a moment.
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// NextSamples drains the ring, waiting when it runs dry - which means the
// simulation itself is running slower than the wall, and a late stream is
// the honest one.
func (g *engineSource) NextSamples(n int) []complex128 {
	for {
		g.mu.Lock()
		if len(g.buf) >= n {
			out := make([]complex128, n)
			copy(out, g.buf[:n])
			g.buf = append(g.buf[:0], g.buf[n:]...)
			g.mu.Unlock()
			return out
		}
		closed := g.closed
		g.mu.Unlock()
		if closed {
			return make([]complex128, n)
		}
		select {
		case <-g.stop:
			return make([]complex128, n)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// sdrServer is one serving observer: the listener and its producer.
type sdrServer struct {
	srv    *sdr.RTLTCP
	src    *engineSource
	rateHz float64
}

func (e *sdrServer) shutdown() {
	_ = e.srv.Close()
	e.src.close()
}

// sdrState holds the servers this session is running, off the Sim struct
// through the DomainState seam. Reached only from the store goroutine - the
// verbs and the per-tick refresh - so it needs no lock of its own.
type sdrState struct {
	servers map[string]*sdrServer
}

func stateOf(s *session.Sim) *sdrState {
	return session.DomainState(s, "sdr", func() *sdrState {
		return &sdrState{servers: map[string]*sdrServer{}}
	})
}

// sources is what is currently served, for the observer windows: address, rate,
// and whether a client is on the line right now.
func sources(ss *sdrState) []state.SDRSource {
	out := make([]state.SDRSource, 0, len(ss.servers))
	for name, e := range ss.servers {
		out = append(out, state.SDRSource{
			Node: name, Addr: e.srv.Addr(), RateHz: e.rateHz,
			Attached: e.srv.Attached(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

func registerSDRServe(st *state.Store, s *session.Sim) {
	st.HandleSpec("sdr.serve", state.Spec{
		What: "offer what one node's antenna hears as an rtl_tcp source, so real " +
			"SDR software can be pointed at the simulated spectrum rather than " +
			"at a drawing of it",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node whose antenna is served; refused when it is " +
					"absent, not in the scenario, or not in the engine"},
		},
		Returns: []string{"node", "addr", "rate_hz"},
		Answers: "`rate_hz` is the node's own receiver bandwidth, one sample per " +
			"hertz, and 250 kHz where the scenario states none. It is what the " +
			"stream is rendered at, not what a client is held to: the client's " +
			"own rate setting is followed. Serving a node already served replaces " +
			"the listener. The IQ is signal only, with the noise floor added at " +
			"the server, so a paused run streams a bare floor rather than stopping.",
		Example: &state.Example{
			Params: "West Lomond", What: "point SDR software at a node's antenna",
		},
	}, func(w *state.World, p any) (any, error) {
		if s.Engine() == nil {
			return nil, session.ErrNoSimulation
		}
		name, _ := session.StringField(p, "node")
		idx := -1
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				idx = i
			}
		}
		if idx < 0 {
			return nil, session.NoSuchNode(name)
		}
		ss := stateOf(s)
		if old, ok := ss.servers[name]; ok {
			old.shutdown()
			delete(ss.servers, name)
		}
		en, ok := s.Engine().NodeByName(name)
		if !ok {
			return nil, fmt.Errorf("%s is not in the engine", name)
		}
		rate := en.Spec().Radio.BandwidthHz
		if rate <= 0 {
			rate = 250e3
		}
		src := newEngineSource(s, idx, rate, s.Engine().ObserverNoisePSD(idx))
		srv, err := sdr.ServeRTLTCP("127.0.0.1:0", src)
		if err != nil {
			src.close()
			return nil, err
		}
		ss.servers[name] = &sdrServer{srv: srv, src: src, rateHz: rate}
		w.SDRSources = sources(ss)
		w.Say(fmt.Sprintf("%s is an rtl_tcp source at %s - the stream follows "+
			"the client's own rate setting (native %.0f Hz)", name, srv.Addr(), rate))
		return map[string]any{"node": name, "addr": srv.Addr(), "rate_hz": rate}, nil
	})

	st.HandleSpec("sdr.stop", state.Spec{
		What: "close a node's rtl_tcp listener and stop rendering its IQ, which " +
			"is work a run keeps doing for as long as it is served",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the served node; refused when it is absent or not being " +
					"served, so a stop is never mistaken for having worked"},
		},
		Returns: []string{"stopped"},
		Answers: "Any client on the line is cut rather than told, the same way " +
			"unplugging a dongle would.",
		Example: &state.Example{
			Params: "West Lomond", What: "stop serving a node's antenna",
		},
	}, func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "node")
		ss := stateOf(s)
		e, ok := ss.servers[name]
		if !ok {
			return nil, fmt.Errorf("%s is not being served", name)
		}
		e.shutdown()
		delete(ss.servers, name)
		w.SDRSources = sources(ss)
		w.Say("stopped serving " + name)
		return map[string]any{"stopped": name}, nil
	})
}
