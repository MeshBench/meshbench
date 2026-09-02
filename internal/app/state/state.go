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
	// Project is the fixture or project this session has open.
	Project string
	// Jobs are long operations in flight, so the interface can show them and
	// offer to cancel rather than appearing to have hung.
	Jobs []Job
	// Status is the most recent line for the status bar, and Log keeps the
	// last few so a message cannot scroll away before it is read. FullLog is
	// the same lines, kept much further back, for a panel that exists to be
	// scrolled rather than glanced at.
	Status  string
	Log     []string
	FullLog []string
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
	// SDRSources are the observers currently served as rtl_tcp sources.
	SDRSources []SDRSource
	// CoverageCells is the coverage raster's long edge; zero means default.
	CoverageCells int
	// RealtimeX is how fast the run is moving against the wall clock: 1 is
	// realtime, 0 means not playing or not yet measured.
	RealtimeX float64
	// Routes are the planner's last answer.
	Routes []Route
	// Import is the last fetch's description, or nil.
	Import *Import
	// ExcessLossDB is the calibration term the model is running with, and
	// Calibrated says it was fitted against real receptions rather than left
	// at the default. A margin's provenance is part of the margin.
	ExcessLossDB float64
	Calibrated   bool
	// Observed is recent traffic on the real network; Residuals is the model
	// measured against it.
	Observed  []Observed
	Residuals *Residuals
	// BoardMatrix is the hardware capability matrix, one row per board, and
	// BoardMatrixVersion the board-image version it was measured against.
	BoardMatrix        []BoardRow
	BoardMatrixVersion string
	// Resources is everything the application downloads at runtime, as last
	// listed from disk.
	Resources []ResourceRow
	// Licence is the terms last asked for, and the row that asked.
	Licence LicenceText
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
	// TerrainDownloads is whether the application may spend this machine's
	// bandwidth on terrain without asking.
	TerrainDownloads bool
	// Ground is what elevation data the studies here actually have under them,
	// as last looked at.
	Ground Ground
	// Setup is what this machine has and has not, as last checked.
	Setup []SetupGroup
	// Update is what the last update check found, empty until one is asked for.
	Update Update
	// Builds is the firmware library on this machine.
	Builds []Build
	// Experiment is the A/B matrix's summary, and ExperimentWarning is why it
	// is not yet a result.
	Experiment        []ArmSummary
	ExperimentWarning string
	// ExperimentRuns is every cell, so a sweep can be watched rather than
	// waited on; ExperimentVerdict is what it concluded, once it has.
	ExperimentRuns    []RunRow
	ExperimentVerdict string
	// ExperimentArms and ExperimentSenders are what is defined right now.
	//
	// They live here rather than in the panel because the panel is not the only
	// thing that defines them: the control socket does too, and a panel holding
	// its own copy showed "no arms yet" over a session with four.
	ExperimentArms    []string
	ExperimentSenders []string
	// Series is the selected node's history, for its graphs.
	Series NodeSeries
	// Provisioning is the script for the node last asked about.
	Provisioning     []ProvisionLine
	ProvisioningNode string
	// Console is one node's firmware scrollback.
	Console     []string
	ConsoleNode string
	// Outputs is every node-and-source pane something is currently looking
	// at: a board's serial port, the emulator's own log, the radio model's.
	//
	// Many rather than one, which is what it was. One pane at a time meant
	// two windows on two nodes overwrote each other every tick, and switching
	// source blanked the pane until the next one landed - both of which read
	// as the workbench losing the log. What is on disk was never lost; what
	// was lost was the one slot they were all sharing.
	Outputs []OutputPane
	// Companions are the companion sessions the workbench currently holds,
	// decoded rather than flattened to console text, so the client can draw a
	// channel list and a conversation instead of a terminal.
	Companions []Companion
	// RFMode is which physics decides reception: "calculated" or "waveform".
	RFMode string
	// KeepAbove is whether windows opened from now on stay above the main
	// one - a Linux-only preference, because only there the ask changes what
	// the windows are.
	KeepAbove bool
	// RFRealism is the optional-imperfections switch set, all zero for the
	// kind default.
	RFRealism RFRealism
	// RFEnvironment is the loaded building-tile directory, or "" for bare
	// earth.
	RFEnvironment string
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
	// half-duplex, interference, collision, receiver-busy, floor, and
	// unclassified for a miss whose cause the engine did not establish.
	Class string
}

// EventCounts is the whole run's events by class, for the cards above the
// table - counted by the engine as they happen, never by walking the log.
type EventCounts struct {
	Sent, Received, HalfDuplex, Interference int
	Collision, ReceiverBusy, Floor           int
	Unclassified                             int
}

// Total is every event the run has produced.
func (c EventCounts) Total() int {
	return c.Sent + c.Received + c.HalfDuplex + c.Interference +
		c.Collision + c.ReceiverBusy + c.Floor + c.Unclassified
}

// RFRealism mirrors the engine's realism switches for the RF Simulation
// panel: every field zero means the kind simulator the docs describe.
type RFRealism struct {
	OscPPM        float64
	MultipathDB   float64
	FadingHz      float64
	ImplLossDB    float64
	SaturationDBm float64
}
