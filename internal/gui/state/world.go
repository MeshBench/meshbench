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
	Jobs       []Job
	Status     string
	Log        []string
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
}
