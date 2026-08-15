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
	"fmt"
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
	// RunUntilMs is the simulated time this run stops at; zero means open.
	RunUntilMs uint32
	// StepMs is how much simulated time one tick advances, so the interface
	// can say how fast it is going rather than only whether it is going.
	StepMs uint32
	Seed   uint64
	Nodes  []Node
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
	// Counts is the whole run's events by class, for the cards.
	Counts EventCounts
	// Packet is the transmission last opened, dissected, or nil.
	Packet *Packet
	Scores []Score
	// Waterfall is the last capture, and WaterfallNote is why there is not
	// one. An empty waterfall and a broken one look identical, so the reason
	// travels with the absence.
	Waterfall     *Coverage
	WaterfallNote string
	// Budgets are the two directions of the link last asked about, and
	// LinkProfile the cut-through between that pair.
	Budgets     []Budget
	LinkProfile *Profile
	// Matrix is the sweep last loaded, or nil.
	Matrix *Matrix
	// Energy is the site study last run, or nil.
	Energy *Energy
	// Sends and Assertions are the fixture's schedule and its claims.
	Sends      []Send
	Assertions []Assertion
	// Endpoints are the companions currently served.
	Endpoints []Endpoint
	// Routes are the planner's last answer.
	Routes []Route
	// Import is the last fetch's description, or nil.
	Import *Import
	// Observed is recent traffic on the real network; Residuals is the model
	// measured against it.
	Observed  []Observed
	Residuals *Residuals
	// Stats is per-node cost and traffic, for the node view.
	Stats []NodeStat
	// Library is every build, published or on disk, with what runs it.
	Library []FirmwareRow
	// GPU is what hardware there is and what the last warm did with it.
	GPU GPUState
	// TileCacheGB bounds the decoded terrain tiles held in memory, and
	// TileCacheDir is where they live on disk.
	TileCacheGB  float64
	TileCacheDir string
	// Builds is the firmware library on this machine.
	Builds []Build
	// Experiment is the A/B matrix's summary, and ExperimentWarning is why it
	// is not yet a result.
	Experiment        []ArmSummary
	ExperimentWarning string
	// ExperimentID is what the currently defined sweep hashes to, and
	// ExperimentIdentity is what went into that hash - so an operator can see,
	// before pressing run, exactly what two people would need to agree on to
	// get the same ID.
	ExperimentID       string
	ExperimentIdentity ExperimentIdentity
	// ExperimentNarrow is the caption for the loosest arm's interval - what
	// it spreads, how many more seeds would tighten it, and about how long
	// that takes - and ExperimentNarrowSeeds is that seed count, ready to hand
	// straight to experiment.extend. Empty when nothing has run yet.
	ExperimentNarrow      string
	ExperimentNarrowSeeds int
	// Regressions is the last directory run's outcome, and RegressionsDir
	// is what ran - so an operator opening the panel sees what it was last
	// pointed at, not just an empty table.
	Regressions    []RegressionResult
	RegressionsDir string
	// Series is the selected node's history, for its graphs.
	Series NodeSeries
	// Provisioning is the script for the node last asked about.
	Provisioning     []ProvisionLine
	ProvisioningNode string
	// Console is one node's firmware scrollback.
	Console     []string
	ConsoleNode string
	// FleetReplies is what each node said to the last fleet command. A
	// command sent to forty nodes with no reply shown is indistinguishable
	// from one that went nowhere.
	FleetReplies []FleetReply
	// FleetCommand is the command they are replies to.
	FleetCommand string
	// RealFirmware is whether play starts MeshCore on every node.
	RealFirmware bool
	// FirmwareRunning is how many nodes have a process up, and
	// FirmwareStarting reports a start in progress.
	FirmwareRunning  int
	FirmwareStarting bool
	// PendingPlay is a run waiting for its firmware to come up.
	PendingPlay bool
}

// Profile is the cut-through between one pair: the ground with the earth's
// curvature in it, the sight line, the first Fresnel zone, and the knife
// edges with what each costs. The picture wb1's Link tab drew.
type Profile struct {
	From, To   string
	DistanceKm float64
	// AtoB and BtoA are the one-way margins, so the chart carries the same
	// numbers the budget does.
	AtoB, BtoA  float64
	Samples     []ProfileSample
	Edges       []ProfileEdge
	Verdict     string
	LowM, HighM float64
}

// ProfileSample is one point along the path, in metres.
type ProfileSample struct {
	DistM    float64
	BulgedM  float64
	LOSm     float64
	FresnelM float64
}

// ProfileEdge is one obstruction and its own Bullington contribution -
// presented as a decomposition, never as an addition.
type ProfileEdge struct {
	DistM  float64
	LossDB float64
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

// FleetReply is one node's answer to a fleet command, in the firmware's own
// words.
type FleetReply struct {
	Node  string
	Reply string
}

// GPUState is the graphics hardware, whether it is being used, and what it
// last did.
//
// What it last did rather than only whether it is switched on: "GPU
// acceleration: on" over a run that quietly fell back to the cores is exactly
// the kind of claim this project does not make.
type GPUState struct {
	Enabled bool
	Present bool
	Device  string
	Backend string
	// Why is the reason there is no GPU, or the reason the last warm did not
	// use the one there is.
	Why string
	// Used, Pairs, CellM and Ms describe the last warm.
	Used  bool
	Pairs int
	CellM float64
	Ms    int64
}

// FirmwareRow is one build in the library: published, on disk, or both.
//
// Merged rather than two lists, because a build imported from a branch is in
// no catalogue and is exactly the kind of thing worth testing, while a
// published one that has never been fetched still has to be offerable.
type FirmwareRow struct {
	Role    string
	Version string
	// Board is empty for a host build. A build for a board runs as emulated
	// hardware, which costs an emulator per node, so the two are never mixed
	// up silently.
	Board  string
	Bytes  int64
	OnDisk bool
	// InUse is how many nodes in this scenario run it, so a delete can say
	// what it would break.
	InUse int
	// Unavailable marks a row that exists only because nodes are pinned to
	// it: nothing on disk, and nothing published this machine could run.
	//
	// Without this such a row looks like any other build until somebody tries
	// to start it, and then vanishes the moment the nodes are repointed -
	// which reads as the library losing builds rather than as a pin nobody
	// can honour.
	Unavailable bool
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
// for something only the packet view ever opens; it asks the store for the
// one packet somebody clicked.
type Event struct {
	AtMs      uint32
	Kind      string
	From, To  string
	MessageID uint64
	PacketID  uint64
	SNRdB     float64
	Detail    string
	// Class buckets the event for the cards and chips: sent, received,
	// half-duplex, interference, floor.
	Class string
}

// EventCounts is the whole run's events by class, for the cards above the
// table - counted by the engine as they happen, never by walking the log.
type EventCounts struct {
	Sent, Received, HalfDuplex, Interference, Floor int
}

// Total is every event the run has produced.
func (c EventCounts) Total() int {
	return c.Sent + c.Received + c.HalfDuplex + c.Interference + c.Floor
}

// Packet is one transmission, dissected, with everywhere it went - the view a
// real capture cannot produce, because no observer is everywhere. Built by
// packet.open when an event is clicked.
type Packet struct {
	ID        uint64
	MessageID uint64
	Origin    string
	AtMs      uint32
	Heard     int
	Missed    int
	// Malformed is the dissection's complaint, empty when the frame parsed.
	Malformed string
	// The header, in the dissector's words.
	RouteType   string
	PayloadType string
	Version     string
	Transport   string
	// Path is the hop hashes, resolved to node names where the run knows
	// them - approximate by construction and labelled where it fails.
	Path []string
	// PayloadFields are what the payload carries in clear; PayloadNote is
	// what to say when it carries nothing readable.
	PayloadFields []PacketField
	PayloadNote   string
	// RawLines is the frame as a formatted hex dump, one line per 16 bytes.
	RawLines []string
	// Fates is what happened at every node that logged an event for this
	// packet; Ledger is the radio-level truth for every receiver.
	Fates  []PacketFate
	Ledger []PacketReception
	// Journey follows the message this packet carried across every relay.
	Journey       []PacketHop
	Transmissions int
	Reached       int
}

// PacketField is one dissected name and value.
type PacketField struct{ Name, Value string }

// PacketFate is one node's outcome for one packet.
type PacketFate struct {
	AtMs  uint32
	Node  string
	Kind  string
	SNRdB float64
	What  string
}

// PacketReception is one row of the reception ledger: what one receiver's
// radio made of one packet, whether or not its firmware ever knew.
type PacketReception struct {
	Node     string
	From     string
	Offered  bool
	RSSIdBm  float64
	SNRdB    float64
	Demod    bool
	CRCOK    bool
	Firmware string // accepted, dropped, never saw it
}

// PacketHop is one transmission of the followed message.
type PacketHop struct {
	AtMs  uint32
	By    string
	Hops  int
	Heard []string
	// MissedBy names who could not decode this relay; Missed keeps the count
	// for callers that only need the number.
	MissedBy []string
	Missed   int
	PacketID uint64
}

// ArmSummary is one arm of an experiment, averaged over its seeds.
type ArmSummary struct {
	Arm       string
	Runs      int
	TX        float64
	RX        float64
	Delivered float64
	Redundant float64
	// RXSpread is the range of receptions across seeds. Zero means every seed
	// returned the same number, which is one draw repeated rather than a
	// spread - and a difference between arms cannot be called larger than a
	// noise nobody has measured.
	RXSpread float64
	// RXLo and RXHi are the 95% Student's t interval on receptions; HasCI is
	// false below two seeds, where there is no spread to build one from.
	RXLo, RXHi float64
	HasCI      bool
	// DeltaPct is against the baseline arm, and HasDelta is false for the
	// baseline arm itself. Verdict is "significant", "not yet", or "" for
	// the baseline - significant means the intervals do not overlap, nothing
	// weaker.
	DeltaPct float64
	HasDelta bool
	Verdict  string
	// PayloadAirtimeMs, OverheadAirtimeMs and RedundantAirtimeMs split the
	// arm's average airtime three ways - see engine.Node for what each means.
	PayloadAirtimeMs   float64
	OverheadAirtimeMs  float64
	RedundantAirtimeMs float64
}

// ExperimentIdentity is what the sweep's ID is a hash of, shown so an
// operator can see what two people would need to agree on before comparing
// their IDs - and what changed, when a replayed ID no longer matches.
type ExperimentIdentity struct {
	Fixture    string
	GeometryFP string
	Firmware   []string
	MeshBench  string
	Dirty      bool
}

// RegressionResult is one regression case's outcome: pass, flag, fail, or
// error (the case itself could not be loaded or run). Detail is empty for a
// pass and always populated otherwise - a red row with no reason sends
// someone back to the log this exists to replace.
type RegressionResult struct {
	Name    string
	Verdict string
	Detail  string
	Seeds   int
	TookMs  float64
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

// Score is one node's counters.
type Score struct {
	Name           string
	Sent           int
	Heard          int
	AirtimeMs      float64
	DutyCyclePct   float64
	UniqueDelivery int
	RedundantRelay int
	// AirtimePayloadMs, AirtimeOverheadMs and AirtimeRedundantMs split
	// AirtimeMs three ways - payload that reached somewhere new, protocol
	// bytes that were never going to deliver a message, and payload relayed
	// to receivers who already had it. The three always sum to AirtimeMs.
	AirtimePayloadMs   float64
	AirtimeOverheadMs  float64
	AirtimeRedundantMs float64
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

// NodeStat is what one node is costing and doing right now.
//
// Separate from Node because Node is what a network *is* and this is what it is
// *doing*: one changes when somebody edits the scenario, the other changes
// every tick, and merging them would republish the whole network every time a
// counter moved.
type NodeStat struct {
	Name string
	// Backend is "native", "emulated" or "" for a node running nothing.
	Backend string
	// Firmware is the build it is running.
	Firmware string
	Running  bool
	// State is what the node is doing: running, stopped, or one of the
	// transitions - stopping, provisioning, starting. A boolean cannot say
	// "changing firmware", and a row that goes blank while it happens looks
	// like a node that has died.
	State string
	// PID, and what the process is costing. RSSBytes is resident memory;
	// CPUPct is a share of one core since the last sample.
	PID      int
	RSSBytes int64
	// CPUPct is a share of one core since the last sample, and CPUms is the
	// total processor time this node has used since it started.
	//
	// Both, because they answer different questions and the percentage alone
	// is unreadable: a node quietly ticking over reads 0.3%, and fifty of them
	// reading 0.3% tells you nothing about which has done the most work. The
	// total does, and it needs no delta - so it is also immune to the startup
	// burst that makes a freshly attached node read high.
	CPUPct float64
	CPUms  int64

	// Sent and Heard are packets; LastSentMs and LastHeardMs are when, in
	// simulated time. Zero means never, which is why they are separate from
	// the counts rather than inferred from them.
	Sent, Heard               int
	LastSentMs, LastHeardMs   uint32
	LastSentTo, LastHeardFrom string

	// The chip's own counters, which are the only way to tell a busy mesh from
	// a radio that cries busy too readily.
	IRQReads, BusyReads uint32
	BusyMs, Spurious    uint32
}

// NodeSeries is one node's recent history, for its graphs.
//
// Oldest first, so drawing it left to right is drawing it forwards.
type NodeSeries struct {
	Name string
	RSS  []int64
	CPU  []float64
	Sent []int
}

// ProvisionLine is one console line a node is sent before a run, and why.
type ProvisionLine struct {
	Command string
	Why     string
	// Comment marks a line that is not sent, so a reader can tell the script
	// from its annotations.
	Comment bool
}

// Node is one node, as the interface needs it.
type Node struct {
	Name     string
	Kind     string
	Lat, Lon float64
	HeightM  float64
	TxDBm    float64
	Regions  []string
	// DefaultScope is the region this node originates under, which the map
	// draws as the innermost ring.
	DefaultScope string
	Firmware     string
	Sent         int
	Heard        int
	Selected     bool
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
	// RunUntilMs stops the run at a simulated time. Zero means run until
	// somebody says otherwise. A run bounded in simulated time is the only
	// kind a script can wait for: wall time says nothing about how far the
	// simulation actually got.
	RunUntilMs uint32
	Seed       uint64
	Nodes      []Node
	Jobs       []Job
	Status     string
	Log        []string
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
	// Counts is the whole run's events by class, for the cards.
	Counts EventCounts
	// Packet is the transmission last opened, dissected, or nil.
	Packet *Packet
	Scores []Score
	// Waterfall is the last capture; WaterfallNote is why there is not one.
	Waterfall     *Coverage
	WaterfallNote string
	// Budgets are the two directions of the link last asked about, and
	// LinkProfile the cut-through between that pair.
	Budgets     []Budget
	LinkProfile *Profile
	// Matrix is the sweep last loaded, or nil.
	Matrix *Matrix
	// Energy is the site study last run, or nil.
	Energy *Energy
	// Sends and Assertions are the fixture's schedule and its claims.
	Sends      []Send
	Assertions []Assertion
	// Endpoints are the companions currently served.
	Endpoints []Endpoint
	// Routes are the planner's last answer.
	Routes []Route
	// Import is the last fetch's description, or nil.
	Import *Import
	// Observed is recent traffic on the real network; Residuals is the model
	// measured against it.
	Observed  []Observed
	Residuals *Residuals
	// Stats is per-node cost and traffic, for the node view.
	Stats []NodeStat
	// Library is every build, published or on disk, with what runs it.
	Library []FirmwareRow
	// GPU is what hardware there is and what the last warm did with it.
	GPU GPUState
	// TileCacheGB bounds the decoded terrain tiles held in memory, and
	// TileCacheDir is where they live on disk.
	TileCacheGB  float64
	TileCacheDir string
	// Builds is the firmware library on this machine.
	Builds []Build
	// Experiment is the A/B matrix's summary, and ExperimentWarning is why it
	// is not yet a result.
	Experiment        []ArmSummary
	ExperimentWarning string
	// ExperimentID is what the currently defined sweep hashes to, and
	// ExperimentIdentity is what went into that hash - so an operator can see,
	// before pressing run, exactly what two people would need to agree on to
	// get the same ID.
	ExperimentID       string
	ExperimentIdentity ExperimentIdentity
	// ExperimentNarrow is the caption for the loosest arm's interval - what
	// it spreads, how many more seeds would tighten it, and about how long
	// that takes - and ExperimentNarrowSeeds is that seed count, ready to hand
	// straight to experiment.extend. Empty when nothing has run yet.
	ExperimentNarrow      string
	ExperimentNarrowSeeds int
	// Regressions is the last directory run's outcome, and RegressionsDir
	// is what ran - so an operator opening the panel sees what it was last
	// pointed at, not just an empty table.
	Regressions    []RegressionResult
	RegressionsDir string
	// Series is the selected node's history, for its graphs.
	Series NodeSeries
	// Provisioning is the script for the node last asked about.
	Provisioning     []ProvisionLine
	ProvisioningNode string
	// Console is one node's firmware scrollback.
	Console     []string
	ConsoleNode string
	// FleetReplies is what each node said to the last fleet command. A
	// command sent to forty nodes with no reply shown is indistinguishable
	// from one that went nowhere.
	FleetReplies []FleetReply
	// FleetCommand is the command they are replies to.
	FleetCommand string
	// RealFirmware is what kind of run this is: on, play starts one MeshCore
	// process per node and every relay decision is the firmware's own. Off,
	// the channel and the collisions are still real but nothing decides to
	// relay. It is a property of the run, set once, and play honours it -
	// rather than a second button that has to be pressed first, in the right
	// order, or the run is a different simulation than intended.
	RealFirmware bool
	// FirmwareRunning is how many nodes have a process up.
	FirmwareRunning int
	// FirmwareStarting reports a start in progress.
	FirmwareStarting bool
	// PendingPlay is a run waiting for its firmware to come up. The clock
	// must not advance over a mesh that is still attaching.
	PendingPlay bool

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
		Areas:    areas,
		MarginKm: s.world.MarginKm,
		Links:    links,
		Trails:   trails,
		Coverage: s.world.Coverage,
		Shade:    s.world.Shade,
		// Events and scores are already rebuilt fresh on every tick, so they
		// are handed over rather than copied again.
		Events:                s.world.Events,
		EventTotal:            s.world.EventTotal,
		Counts:                s.world.Counts,
		Packet:                s.world.Packet,
		Scores:                s.world.Scores,
		Waterfall:             s.world.Waterfall,
		WaterfallNote:         s.world.WaterfallNote,
		Budgets:               s.world.Budgets,
		LinkProfile:           s.world.LinkProfile,
		Matrix:                s.world.Matrix,
		Energy:                s.world.Energy,
		Sends:                 s.world.Sends,
		Assertions:            s.world.Assertions,
		Endpoints:             s.world.Endpoints,
		Routes:                s.world.Routes,
		Import:                s.world.Import,
		Observed:              s.world.Observed,
		Residuals:             s.world.Residuals,
		Stats:                 s.world.Stats,
		Builds:                s.world.Builds,
		Library:               append([]FirmwareRow(nil), s.world.Library...),
		GPU:                   s.world.GPU,
		TileCacheGB:           s.world.TileCacheGB,
		TileCacheDir:          s.world.TileCacheDir,
		Experiment:            s.world.Experiment,
		ExperimentWarning:     s.world.ExperimentWarning,
		ExperimentID:          s.world.ExperimentID,
		ExperimentIdentity:    s.world.ExperimentIdentity,
		ExperimentNarrow:      s.world.ExperimentNarrow,
		ExperimentNarrowSeeds: s.world.ExperimentNarrowSeeds,
		Regressions:           append([]RegressionResult(nil), s.world.Regressions...),
		RegressionsDir:        s.world.RegressionsDir,
		Series:                s.world.Series,
		Provisioning:          s.world.Provisioning,
		Console:               s.world.Console,
		FleetReplies:          append([]FleetReply(nil), s.world.FleetReplies...),
		FleetCommand:          s.world.FleetCommand,
		RealFirmware:          s.world.RealFirmware,
		FirmwareRunning:       s.world.FirmwareRunning,
		FirmwareStarting:      s.world.FirmwareStarting,
		ConsoleNode:           s.world.ConsoleNode,
		ProvisioningNode:      s.world.ProvisioningNode,
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
