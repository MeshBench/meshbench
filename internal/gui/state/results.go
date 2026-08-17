// What a run produced, in the shapes the panels read: sweeps, budgets,
// energy, routes, imports and the measurements taken against reality.
package state

import "time"

// ArmSummary is one arm of an experiment, averaged over its seeds.
type ArmSummary struct {
	Arm       string
	Runs      int
	TX        float64
	RX        float64
	Delivered float64
	Redundant float64
	Collided  float64
	AirtimeMs float64
	// RXSpread is how much the seeds of this arm disagree, as a fraction of
	// its own mean: half the range, so it reads as a ±. Zero means every seed
	// returned the same number, which is one draw repeated rather than a
	// spread - and a difference between arms cannot be called larger than a
	// noise nobody has measured.
	RXSpread float64
	// PerSecond is receptions in each second after the burst, summed over this
	// arm's seeds. The shape of the flood rather than its total.
	PerSecond []int
}

// RunRow is one cell of the matrix: an arm at a seed, and where it has got to.
//
// Per run rather than per arm because a sweep is watched while it runs, and an
// arm summary says nothing until every seed of it has finished.
type RunRow struct {
	Arm   string
	Seed  uint64
	State string // queued, running, done, failed
	// Result is what came back, or why nothing did.
	Result string
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

// Send is one scheduled line at a node, from the fixture.
type Send struct {
	Node    string
	AtMs    uint32
	EveryMs uint32
	Command string
}

// Assertion is one claim the fixture makes about a run.
type Assertion struct {
	Kind     string
	Node     string
	WithinMs uint32
	AtLeast  int
	AtMost   int
	MaxPct   float64
}

// Endpoint is one companion served to a client.
type Endpoint struct {
	Node     string
	Kind     string
	Addr     string
	Attached bool
}

// Route is one way to connect two points, from the planner.
type Route struct {
	NewSites     int
	Hops         int
	LongestHopKm float64
	Through      string
}

// Import is what a fetch found, before anything is committed.
type Import struct {
	URL               string
	Records           int
	Nodes             int
	SkippedNoPosition int
	SkippedOutside    int
	Uncertain         int
	Participants      int
}

// Observed is one reception on the real network.
type Observed struct {
	At       time.Time
	Receiver string
	Origin   string
	HopCount int
	HasSNR   bool
	SNRdB    float64
	PacketID string
}

// Residuals is the model measured against those receptions.
//
// A bias and a spread rather than a verdict: "3 dB optimistic on this network"
// is something somebody can correct for, and "validation failed" is not.
type Residuals struct {
	Matched   int
	Unmatched int
	MedianDB  float64
	IQRdB     float64
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

// Build is one firmware image on this machine.
//
// What is in the cache is the only thing that decides what a node can run: a
// build that failed to download and one in daily use look identical from
// anywhere else.
type Build struct {
	Native  bool
	Version string
	Role    string
	Board   string
	Bytes   int64
	Path    string
}
