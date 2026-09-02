// The world: everything the application knows, owned by the store's
// goroutine and copied into a snapshot for the renderer.
package state

import (
	"fmt"
	"io"
	"time"
)

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
	// Project is the fixture or project this session has open, as it was
	// named when it was opened. Kept because it is how a person tells two
	// running sessions apart, and the status line it used to be the only
	// record of is overwritten by the next thing that happens.
	Project string
	// Resources is everything the application downloads at runtime, as last
	// listed from disk.
	Resources []ResourceRow
	// Licence is the terms last asked for, and the row that asked. On the
	// world rather than on every row because they are read on demand: a list
	// carrying every licence it might one day show has opened files nobody
	// asked to see.
	Licence LicenceText
	Jobs    []Job
	Status  string
	Log     []string
	// FullLog is Log kept much further back - a few thousand lines rather
	// than twenty - for a panel built to be scrolled and searched, not just
	// glanced at.
	FullLog []string
	// logWriter, when set, gets every status line too - timestamped and
	// unbounded, unlike either Log or FullLog. Set once before Run starts;
	// nothing else touches World before then.
	logWriter io.Writer
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
	// Calibrated says it was fitted rather than left at the default.
	ExcessLossDB float64
	Calibrated   bool
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
	// TerrainDownloads is whether the application may spend this machine's
	// bandwidth on terrain without asking. Here as well as in the settings
	// file because the switch that grants it has to be able to draw its own
	// position, and because a study held up waiting for it is a state the
	// interface has to be able to explain.
	TerrainDownloads bool
	// Ground is what elevation data the studies here actually have under them,
	// as last looked at. Here rather than computed in the renderer because the
	// caveat line in the chrome has to be able to say "no terrain" without
	// stat-ing a tile cache once a frame.
	Ground Ground
	// Update is what the last update check found. Empty until something asks:
	// nothing fills it at startup, because a check nobody asked for is what
	// this was designed not to be.
	Update Update
	// Setup is what this machine has and has not, as last checked. Grouped
	// rather than flat because what is true of a whole group - where tools are
	// looked for, and that it is not PATH - is the half people get wrong.
	Setup []SetupGroup
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
	// BoardMatrix is the hardware capability matrix: one row per board,
	// each capability three-valued - untested, passed or failed with a
	// reason - never blank, because a blank cell reads as working.
	BoardMatrix        []BoardRow
	BoardMatrixVersion string
	// Series is the selected node's history, for its graphs.
	Series NodeSeries
	// Provisioning is the script for the node last asked about.
	Provisioning     []ProvisionLine
	ProvisioningNode string
	// Console is one node's firmware scrollback.
	Console     []string
	ConsoleNode string
	// Outputs is every node-and-source pane something is currently looking
	// at - what a serial port printed, what the emulator itself said, what
	// the radio model logged. Separate from Console, which is the scrollback
	// of a conversation: these are files on disk, read whole and shown as
	// they are.
	//
	// A list rather than one, which is what it was. One slot meant two
	// windows on two nodes overwrote each other every tick and switching
	// source blanked the pane until the next one landed - both of which read
	// as the workbench losing the log. Nothing on disk was ever lost; the one
	// slot they shared was.
	Outputs []OutputPane
	// Companions is every companion session, decoded.
	Companions []Companion
	// FleetReplies is what each node said to the last fleet command. A
	// command sent to forty nodes with no reply shown is indistinguishable
	// from one that went nowhere.
	FleetReplies []FleetReply
	// FleetCommand is the command they are replies to.
	FleetCommand string
	// RFMode is which physics decides reception: "calculated" or "waveform".
	RFMode string
	// KeepAbove is the machine preference for whether panels in their own
	// windows stay above the main one. A preference rather than a fact of
	// the interface because only Linux pays anything for it: there the ask
	// is a Wayland layer-shell window, which carries no decoration of the
	// compositor's, so the windows themselves must draw their title bars.
	// Windows opened before a change keep what they were made as.
	KeepAbove bool
	// RFRealism is the optional-imperfections switch set.
	RFRealism RFRealism
	// RFEnvironment is the loaded building-tile directory, or "" for bare
	// earth.
	RFEnvironment string
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

func (w *World) Say(msg string) {
	w.Status = msg
	w.Log = append(w.Log, msg)
	if len(w.Log) > 20 {
		w.Log = w.Log[len(w.Log)-20:]
	}
	line := fmt.Sprintf("%s  t=%8.3fs  %s",
		time.Now().Format("15:04:05.000"), float64(w.NowMs)/1000, msg)
	w.FullLog = append(w.FullLog, line)
	if len(w.FullLog) > maxFullLog {
		w.FullLog = w.FullLog[len(w.FullLog)-maxFullLog:]
	}
	if w.logWriter != nil {
		_, _ = fmt.Fprintln(w.logWriter, line)
	}
}

// Job is one long-running operation.
type Job struct {
	ID       string
	What     string
	Done     int
	Total    int
	Cancel   func()
	Finished bool
	// Failed marks a job that ended without doing what it was for.
	//
	// Separate from Finished because a waiter needs both: "stop waiting" and
	// "this did not work" are different answers, and the only way to tell them
	// apart used to be reading the What line - which means matching on prose,
	// and breaks the moment somebody improves the wording.
	Failed bool
}

// JobsRunning is how many jobs are still in flight.
//
// A finished job is deliberately left in the list by some of the things that
// end one, so a caller polling at the wrong moment still learns how what it
// was waiting for turned out. That makes len(Jobs) the wrong answer to "is
// anything happening": it stayed at two for the rest of a session after a
// download that had already landed on disk.
func (w *World) JobsRunning() int {
	n := 0
	for i := range w.Jobs {
		if !w.Jobs[i].Finished {
			n++
		}
	}
	return n
}
