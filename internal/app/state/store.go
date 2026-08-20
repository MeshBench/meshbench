// The store itself: one goroutine owning the world, verbs as messages to it,
// and immutable snapshots out.
//
// Verbs are serviced here rather than on the frame thread, which is the whole
// point of the design - a slow verb cannot stall a frame, and a slow frame
// cannot delay a verb.
package state

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Handler runs one verb. It is called on the store's goroutine, so it may read
// and write state freely and must not block for long: anything slow should
// start a Job and return.
type Handler func(w *World, params any) (any, error)

// Store is the state layer.
type Store struct {
	mu       sync.Mutex
	handlers map[string]Handler

	cmds chan cmd
	snap atomic.Pointer[Snapshot]

	world World
	seq   uint64

	stop chan struct{}
	done chan struct{}

	// stepMs is how much simulated time one tick advances. It is deliberately
	// independent of the frame rate: the simulation must not run faster on a
	// machine with a better graphics card.
	stepMs uint32
}

type cmd struct {
	verb   string
	params any
	reply  chan result
}

type result struct {
	value any
	err   error
}

// ErrUnknownVerb is returned for a verb with no handler, by name, so a caller
// sees which one rather than a generic failure.
var ErrUnknownVerb = errors.New("state: unknown verb")

// New creates a store. It does not run until Run is called.
func New(stepMs uint32) *Store {
	s := &Store{
		handlers: map[string]Handler{},
		cmds:     make(chan cmd, 64),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		stepMs:   stepMs,
	}
	// Real firmware is what this simulator is for, so it is the assumption
	// rather than a switch to be found first. Turning it off is a deliberate
	// choice - a channel with no firmware behind it - and reads as one.
	s.world.RealFirmware = true
	s.publish()
	return s
}

// Handle registers a verb. Registering twice replaces, which is what a test
// wants and what a plugin would want.
func (s *Store) Handle(verb string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[verb] = h
}

// Verbs lists what is registered, so the parity test can be generated from the
// store rather than from a document that has already been wrong once.
func (s *Store) Verbs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.handlers))
	for v := range s.handlers {
		out = append(out, v)
	}
	return out
}

// Run owns the state until the context ends. Everything below this line runs
// on this goroutine and nowhere else.
func (s *Store) Run(ctx context.Context) {
	defer close(s.done)
	tick := time.NewTicker(time.Duration(s.stepMs) * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case c := <-s.cmds:
			s.mu.Lock()
			h, ok := s.handlers[c.verb]
			s.mu.Unlock()
			if !ok {
				// Named. The comment on ErrUnknownVerb has always said "by
				// name" and the error has never carried one, so a typo at a
				// socket returned "unknown verb" and left somebody to work out
				// which of forty it was.
				c.reply <- result{nil, fmt.Errorf("%w: %q", ErrUnknownVerb, c.verb)}
				continue
			}
			v, err := h(&s.world, c.params)
			s.publish()
			c.reply <- result{v, err}
		case <-tick.C:
			// Answer everything waiting before stepping.
			//
			// A step with fifty-eight real firmware processes behind it takes
			// far longer than the tick interval, so the ticker is always ready
			// and select gives a queued verb only even odds against it. That
			// is enough to make the control socket time out while a run is
			// playing - the workbench looks hung precisely when it is working
			// hardest. Commands are cheap; drain them first.
			for drained := 0; drained < 64; drained++ {
				select {
				case c := <-s.cmds:
					h, ok := s.handlers[c.verb]
					if !ok {
						c.reply <- result{nil, fmt.Errorf("%w: %q", ErrUnknownVerb, c.verb)}
						continue
					}
					v, err := h(&s.world, c.params)
					s.publish()
					c.reply <- result{v, err}
				default:
					drained = 64
				}
			}
			// Engine pacing, out of the frame loop. Simulated time advances on
			// this ticker whether or not anything is being drawn, which is what
			// makes a headless run and a watched run the same run.
			if s.world.Playing {
				s.world.NowMs += s.stepMs
				if s.world.Tick != nil {
					s.world.Tick(s.stepMs)
				}
				// Checked after the step, so "run for 10 s" ends at or past
				// ten seconds rather than one tick short of it.
				if s.world.RunUntilMs != 0 && s.world.NowMs >= s.world.RunUntilMs {
					s.world.Playing = false
					s.world.RunUntilMs = 0
					s.world.Say("run finished")
				}
				s.publish()
			}
		}
	}
}

// Close stops the store and waits for it.
func (s *Store) Close() {
	close(s.stop)
	<-s.done
}

// Do runs a verb and waits for its result. Safe from any goroutine: the
// control socket, the MCP server, a test, or the renderer.
func (s *Store) Do(ctx context.Context, verb string, params any) (any, error) {
	reply := make(chan result, 1)
	select {
	case s.cmds <- cmd{verb: verb, params: params, reply: reply}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r := <-reply:
		return r.value, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Snapshot is what the renderer reads. It never blocks, never waits for a
// verb, and never returns a half-applied change.
func (s *Store) Snapshot() *Snapshot { return s.snap.Load() }

// publish freezes the current world into a new snapshot. Called on the store's
// goroutine only.
func (s *Store) publish() {
	s.seq++
	nodes := make([]Node, len(s.world.Nodes))
	copy(nodes, s.world.Nodes)
	jobs := make([]Job, len(s.world.Jobs))
	copy(jobs, s.world.Jobs)
	log := make([]string, len(s.world.Log))
	copy(log, s.world.Log)
	fullLog := make([]string, len(s.world.FullLog))
	copy(fullLog, s.world.FullLog)
	// Links and areas are copied too. A snapshot the renderer may hold for
	// several frames must not alias a slice the store can still append to.
	links := make([]Link, len(s.world.Links))
	copy(links, s.world.Links)
	areas := make([]Area, len(s.world.Areas))
	copy(areas, s.world.Areas)
	trails := make([]Trail, len(s.world.Trails))
	copy(trails, s.world.Trails)
	s.snap.Store(&Snapshot{
		Seq:        s.seq,
		NowMs:      s.world.NowMs,
		Playing:    s.world.Playing,
		RunUntilMs: s.world.RunUntilMs,
		// The declared field was never filled in, so any page saying how fast
		// the run goes read zero and had to guess.
		StepMs:   s.stepMs,
		Seed:     s.world.Seed,
		Nodes:    nodes,
		Jobs:     jobs,
		Status:   s.world.Status,
		Log:      log,
		FullLog:  fullLog,
		Areas:    areas,
		MarginKm: s.world.MarginKm,
		Links:    links,
		Trails:   trails,
		Coverage: s.world.Coverage,
		Shade:    s.world.Shade,
		// Events and scores are already rebuilt fresh on every tick, so they
		// are handed over rather than copied again.
		Events:            s.world.Events,
		EventTotal:        s.world.EventTotal,
		Counts:            s.world.Counts,
		Packet:            s.world.Packet,
		Scores:            s.world.Scores,
		Waterfall:         s.world.Waterfall,
		WaterfallNote:     s.world.WaterfallNote,
		Budgets:           s.world.Budgets,
		LinkProfile:       s.world.LinkProfile,
		Matrix:            s.world.Matrix,
		Energy:            s.world.Energy,
		Sends:             s.world.Sends,
		Assertions:        s.world.Assertions,
		Endpoints:         s.world.Endpoints,
		SDRSources:        s.world.SDRSources,
		CoverageCells:     s.world.CoverageCells,
		RealtimeX:         s.world.RealtimeX,
		Routes:            s.world.Routes,
		Import:            s.world.Import,
		ExcessLossDB:      s.world.ExcessLossDB,
		Calibrated:        s.world.Calibrated,
		Observed:          s.world.Observed,
		Residuals:         s.world.Residuals,
		Resources:         append([]ResourceRow(nil), s.world.Resources...),
		Stats:             s.world.Stats,
		Builds:            s.world.Builds,
		Library:           append([]FirmwareRow(nil), s.world.Library...),
		GPU:               s.world.GPU,
		TileCacheGB:       s.world.TileCacheGB,
		TileCacheDir:      s.world.TileCacheDir,
		Experiment:        s.world.Experiment,
		ExperimentWarning: s.world.ExperimentWarning,
		ExperimentRuns:    s.world.ExperimentRuns,
		ExperimentVerdict: s.world.ExperimentVerdict,
		ExperimentArms:    s.world.ExperimentArms,
		ExperimentSenders: s.world.ExperimentSenders,
		Series:            s.world.Series,
		Provisioning:      s.world.Provisioning,
		Console:           s.world.Console,
		Companions:        s.world.Companions,
		RFMode:            s.world.RFMode,
		RFRealism:         s.world.RFRealism,
		RFEnvironment:     s.world.RFEnvironment,
		FleetReplies:      append([]FleetReply(nil), s.world.FleetReplies...),
		FleetCommand:      s.world.FleetCommand,
		RealFirmware:      s.world.RealFirmware,
		FirmwareRunning:   s.world.FirmwareRunning,
		FirmwareStarting:  s.world.FirmwareStarting,
		ConsoleNode:       s.world.ConsoleNode,
		ProvisioningNode:  s.world.ProvisioningNode,
	})
}

// Say records a status line. Called from a handler.
// maxFullLog is how much of the run's own talk a session keeps in memory for
// the Logs panel - far more than the twenty-line strip, short of holding an
// unbounded run's entire history hostage in RAM.
const maxFullLog = 5000

// SetLogWriter is where every status line also goes, timestamped and kept in
// full - unlike Log, which is only ever the last twenty, for the strip that
// draws it. Set once, before Run starts: nothing else touches World before
// then, so there is nothing here to race.
func (s *Store) SetLogWriter(w io.Writer) {
	s.world.logWriter = w
}

// SetStepMs changes how much simulated time one tick advances.
//
// Called on the store's goroutine, from a verb, so the pacing cannot change
// halfway through a tick.
func (s *Store) SetStepMs(ms uint32) {
	if ms < 1 {
		ms = 1
	}
	if ms > 1000 {
		ms = 1000
	}
	s.stepMs = ms
}

// StepMs is the current tick size.
func (s *Store) StepMs() uint32 { return s.stepMs }
