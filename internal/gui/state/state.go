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
	"image"
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
	// Areas are the study boundaries, and MarginKm the band outside them
	// within which external nodes still matter.
	Areas    []Area
	MarginKm float64
	// Links are the pairs that can hear each other, with the weaker
	// direction's margin. Computed when the network changes rather than per
	// frame: it is an n-squared path loss, and the answer only moves when a
	// node does.
	Links []Link
	// Trails are recent transmissions for the map to fade out.
	Trails []Trail
	// Coverage is the raster last asked for, or nil. Shade is the hillshade
	// for the view it was computed over.
	Coverage *Coverage
	Shade    *Coverage
	// Events is the tail of the engine's log, oldest first, and EventTotal is
	// how many there have been. The tail rather than all of them because a
	// snapshot is copied on every publish and a long run has millions.
	Events     []Event
	EventTotal int
	Scores     []Score
	// Waterfall is the last capture, and WaterfallNote is why there is not
	// one. An empty waterfall and a broken one look identical, so the reason
	// travels with the absence.
	Waterfall     *Coverage
	WaterfallNote string
	// Budgets are the two directions of the link last asked about.
	Budgets []Budget
	// Matrix is the sweep last loaded, or nil.
	Matrix *Matrix
	// Energy is the site study last run, or nil.
	Energy *Energy
}

// Point is a position, and the only geometry the snapshot carries.
type Point struct{ Lat, Lon float64 }

// Area is one study boundary: outer rings, and holes that are outside it.
type Area struct {
	Name  string
	Rings [][]Point
	Holes [][]Point
}

// Link is a pair that can hear each other.
type Link struct {
	// A and B index into Nodes.
	A, B int
	// MarginDB is the weaker direction's margin above what that end needs to
	// decode. Negative is a link that does not close. The weaker direction,
	// because a link that works in one direction only is not a link.
	MarginDB float64
	// AtoB and BtoA are the two directions separately. Carried because the
	// asymmetry between them is a real property of a link - a mast heard by a
	// handheld it cannot answer - and MarginDB, being the weaker of the two,
	// is exactly the number that hides it.
	AtoB, BtoA float64
	// Known is false when nothing has computed a margin yet, which is not the
	// same as a margin of zero and must not be drawn as one.
	Known bool
}

// Trail is one transmission recently on the air, for the map to fade out.
//
// Kept as node indices and a time rather than as a colour and an alpha: how
// old a packet is, is a fact about the run; how faint to draw it is a decision
// about a frame, and the two do not belong in the same place.
type Trail struct {
	// From indexes into Nodes. To is -1 for a transmission nobody received,
	// which is drawn as a stub rather than as a link and is the whole reason
	// this is not a list of links.
	From, To  int
	AtMs      uint32
	Delivered bool
}

// Coverage is a computed raster, ready to draw.
//
// An image rather than cells: a renderer that has to know what a decibel is in
// order to paint a picture is one that will eventually disagree with the panel
// printing the number.
type Coverage struct {
	Node                     string
	Image                    *image.RGBA
	South, North, West, East float64
	// NoDataCells of Cells had no elevation to answer with. Carried because
	// "no coverage" and "no data" look identical on a map and are not the same
	// claim.
	NoDataCells, Cells int
}

// Event is one thing the engine did, as a table needs it.
//
// The frame bytes are deliberately not here. A snapshot is copied on every
// publish, and a hundred thousand events each carrying a frame is real memory
// for something only the inspector ever opens; it asks the store for the frame
// of the one event somebody clicked.
type Event struct {
	AtMs      uint32
	Kind      string
	From, To  string
	MessageID uint64
	PacketID  uint64
	SNRdB     float64
	Detail    string
}

// Score is one node's counters.
type Score struct {
	Name           string
	Sent           int
	Heard          int
	AirtimeMs      float64
	DutyCyclePct   float64
	UniqueDelivery int
	RedundantRelay int
}

// BudgetTerm is one line of a link budget: a named quantity in decibels and
// whether it adds or takes away.
//
// Carried as terms rather than as a total because the total is the one thing
// somebody can already read off the map. What a budget panel is for is which
// term is the reason.
type BudgetTerm struct {
	Name string
	DB   float64
}

// Budget is one direction of one link, broken down.
type Budget struct {
	From, To string
	Terms    []BudgetTerm
	// MarginDB is the running total after every term, and is what the map
	// draws. Kept so the panel and the map cannot disagree by rounding.
	MarginDB float64
}

// Matrix is one metric over arms and seeds.
//
// Values is row-major, arms down and seeds across, and NaN marks a cell that
// was not run. Not zero: a run that did not happen and a run that measured
// nothing are different claims, and a heatmap that draws them the same colour
// tells the reader the wrong one.
type Matrix struct {
	Metric string
	Arms   []string
	Seeds  []uint64
	Values []float64
}

// Energy is a year at one site.
//
// SoC is the daily minimum state of charge, not the daily mean: a pack that
// averages half full and empties every night at three is a pack that does not
// work, and the mean is the number that hides it.
type Energy struct {
	Node         string
	DutyPct      float64
	SoC          []float64
	WorstSoC     float64
	WorstDay     int
	DeadDays     int
	AutonomyDays float64
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
	// Pattern is the antenna's gain in dBi at every 10 degrees of compass
	// bearing, feedline loss already deducted, starting at north.
	//
	// Sampled here rather than in the renderer because the renderer's job is
	// to draw a snapshot, not to know what an antenna is. Nil for a node with
	// no pattern, which is drawn as no overlay rather than as a circle.
	Pattern []float64
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
	// Areas are the study boundaries, and MarginKm the band outside them
	// within which external nodes still matter.
	Areas    []Area
	MarginKm float64
	// Links are the pairs that can hear each other, with the weaker
	// direction's margin. Computed when the network changes rather than per
	// frame: it is an n-squared path loss, and the answer only moves when a
	// node does.
	Links []Link
	// Trails are recent transmissions, newest last. Bounded, because a run
	// that has been going for an hour has more of them than a map can say
	// anything about.
	Trails []Trail
	// Coverage is the raster last asked for, or nil. Shade is the hillshade
	// for the view it was computed over.
	Coverage *Coverage
	Shade    *Coverage
	// Events is the tail of the engine's log; EventTotal counts all of them.
	Events     []Event
	EventTotal int
	Scores     []Score
	// Waterfall is the last capture; WaterfallNote is why there is not one.
	Waterfall     *Coverage
	WaterfallNote string
	// Budgets are the two directions of the link last asked about.
	Budgets []Budget
	// Matrix is the sweep last loaded, or nil.
	Matrix *Matrix
	// Energy is the site study last run, or nil.
	Energy *Energy

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
	// Links and areas are copied too. A snapshot the renderer may hold for
	// several frames must not alias a slice the store can still append to.
	links := make([]Link, len(s.world.Links))
	copy(links, s.world.Links)
	areas := make([]Area, len(s.world.Areas))
	copy(areas, s.world.Areas)
	trails := make([]Trail, len(s.world.Trails))
	copy(trails, s.world.Trails)
	s.snap.Store(&Snapshot{
		Seq:      s.seq,
		NowMs:    s.world.NowMs,
		Playing:  s.world.Playing,
		Seed:     s.world.Seed,
		Nodes:    nodes,
		Jobs:     jobs,
		Status:   s.world.Status,
		Log:      log,
		Areas:    areas,
		MarginKm: s.world.MarginKm,
		Links:    links,
		Trails:   trails,
		Coverage: s.world.Coverage,
		Shade:    s.world.Shade,
		// Events and scores are already rebuilt fresh on every tick, so they
		// are handed over rather than copied again.
		Events:        s.world.Events,
		EventTotal:    s.world.EventTotal,
		Scores:        s.world.Scores,
		Waterfall:     s.world.Waterfall,
		WaterfallNote: s.world.WaterfallNote,
		Budgets:       s.world.Budgets,
		Matrix:        s.world.Matrix,
		Energy:        s.world.Energy,
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
