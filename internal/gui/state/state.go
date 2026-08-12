// Package state owns everything the application knows, on one goroutine, and
// hands the renderer immutable snapshots of it.
//
// This exists because of a specific defect in the old design: control verbs
// were serviced on the frame thread. That made headless mode need its own ADR,
// made screenshots fight the renderer, and meant a sweep could stall a console
// reply. Four verbs had to be special-cased as "polls" so that driving the
// application did not deadlock against the thing being driven.
//
// The shape here removes the possibility rather than managing it:
//
//   - one goroutine owns the state and is the only writer
//   - verbs are messages to it, and a verb that takes a while takes a while
//     without anything else waiting on it
//   - the renderer never reads state, only a snapshot, so a frame cannot tear
//     and a slow frame cannot delay a verb
package state

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Snapshot is an immutable view of the world for one frame.
//
// Every field is either a value or a slice the store will never write to
// again. The renderer may hold one for as long as it likes.
type Snapshot struct {
	// Seq increases on every change, so a renderer can tell whether anything
	// happened without comparing contents.
	Seq uint64
	// NowMs is simulated time, which is not wall time and never has been.
	NowMs uint32
	// Playing reports whether the engine is advancing.
	Playing bool
	Seed    uint64
	Nodes   []Node
	// Jobs are long operations in flight, so the interface can show them and
	// offer to cancel rather than appearing to have hung.
	Jobs []Job
	// Status is the most recent line for the status bar, and Log keeps the
	// last few so a message cannot scroll away before it is read.
	Status string
	Log    []string
}

// Node is one node, as the interface needs it.
type Node struct {
	Name     string
	Kind     string
	Lat, Lon float64
	HeightM  float64
	TxDBm    float64
	Regions  []string
	Firmware string
	Sent     int
	Heard    int
	Selected bool
}

// Job is one long-running operation.
type Job struct {
	ID       string
	What     string
	Done     int
	Total    int
	Cancel   func()
	Finished bool
}

// Handler runs one verb. It is called on the store's goroutine, so it may read
// and write state freely and must not block for long: anything slow should
// start a Job and return.
type Handler func(w *World, params any) (any, error)

// World is the mutable state, only ever touched on the store's goroutine.
type World struct {
	NowMs   uint32
	Playing bool
	Seed    uint64
	Nodes   []Node
	Jobs    []Job
	Status  string
	Log     []string

	// Tick is called every step while playing, and is where engine pacing
	// lives now that it is out of the frame loop. Nil means no engine.
	Tick func(dtMs uint32)
}

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
				c.reply <- result{nil, ErrUnknownVerb}
				continue
			}
			v, err := h(&s.world, c.params)
			s.publish()
			c.reply <- result{v, err}
		case <-tick.C:
			// Engine pacing, out of the frame loop. Simulated time advances on
			// this ticker whether or not anything is being drawn, which is what
			// makes a headless run and a watched run the same run.
			if s.world.Playing {
				s.world.NowMs += s.stepMs
				if s.world.Tick != nil {
					s.world.Tick(s.stepMs)
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
	s.snap.Store(&Snapshot{
		Seq:     s.seq,
		NowMs:   s.world.NowMs,
		Playing: s.world.Playing,
		Seed:    s.world.Seed,
		Nodes:   nodes,
		Jobs:    jobs,
		Status:  s.world.Status,
		Log:     log,
	})
}

// Say records a status line. Called from a handler.
func (w *World) Say(msg string) {
	w.Status = msg
	w.Log = append(w.Log, msg)
	if len(w.Log) > 20 {
		w.Log = w.Log[len(w.Log)-20:]
	}
}
