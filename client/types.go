// The values a caller holds.
//
// Snapshots, all of them: a value read once, which never changes afterwards.
// The live things - the workbench, a node handle - are types with methods, and
// they re-read the session every time they are asked. Which one a thing is
// decides whether it can be held across a Run and still be true, so it is
// stated on each.
package client

import "time"

// Hello is what a connection is talking to. Snapshot, read once at connect.
type Hello struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
	// Mode is "workbench" or "headless".
	Mode   string `json:"mode"`
	Socket string `json:"socket"`
	Verbs  int    `json:"verbs"`
	// PID and StartedAt tell a restart from a reconnect. The scenario does
	// not survive a restart, so a script picking up a session it did not
	// start has to be able to ask.
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// Describe is the cheap summary. Snapshot.
type Describe struct {
	Nodes   int    `json:"nodes"`
	Seed    uint64 `json:"seed"`
	NowMs   uint32 `json:"now_ms"`
	Playing bool   `json:"playing"`
}

// NodeInfo is what a network is, per node. Snapshot: take another with
// Nodes.List when something has changed it.
//
// What a node is *doing* - running, its memory, its counters - is NodeStat,
// because the two change on completely different timescales and the store
// publishes them separately.
type NodeInfo struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Lat      float64  `json:"lat"`
	Lon      float64  `json:"lon"`
	HeightM  float64  `json:"height_m"`
	TxDBm    float64  `json:"tx_dbm"`
	Regions  []string `json:"regions"`
	Firmware string   `json:"firmware"`
	Sent     int      `json:"sent"`
	Heard    int      `json:"heard"`
	Selected bool     `json:"selected"`
}

// Kinds, as the scenario names them.
const (
	SimpleRepeater   = "simple-repeater"
	AdvancedRepeater = "advanced-repeater"
	Companion        = "companion"
	// RoomServer holds posts for clients to collect and does not forward. A
	// mesh that treats one as a repeater overstates its own reach.
	RoomServer = "room-server"
	// SDRObserver runs no firmware and transmits nothing: it captures the
	// summed field at its antenna and hands back IQ.
	SDRObserver = "sdr-observer"
	// Emitter is interference that is not MeshCore, propagated through the
	// same terrain as everything else.
	Emitter = "emitter"
)

// Event is one thing the engine did. Snapshot.
//
// The frame bytes are deliberately absent: a long run has millions of these,
// and the one packet somebody wants is asked for by id.
type Event struct {
	AtMs      uint32 `json:"at_ms"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
	MessageID uint64 `json:"message_id"`
	PacketID  uint64 `json:"packet_id"`
	// SNRdB is null on the wire for an infinite ratio - a reception with no
	// noise at all - so it is a pointer here. Absent is not zero.
	SNRdB  *float64 `json:"snr_db"`
	Detail string   `json:"detail"`
	Class  string   `json:"class"`
}

// Event classes, as the engine buckets them.
const (
	ClassSent         = "sent"
	ClassReceived     = "received"
	ClassHalfDuplex   = "half-duplex"
	ClassInterference = "interference"
	ClassFloor        = "floor"
)

// SimState is the clock. Snapshot.
type SimState struct {
	Playing bool   `json:"playing"`
	NowMs   uint32 `json:"now_ms"`
	UntilMs uint32 `json:"until_ms"`
	Events  int    `json:"events"`
	StepMs  uint32 `json:"step_ms"`
	Seed    uint64 `json:"seed"`
}

// FirmwareState is how far a start has got. Snapshot.
type FirmwareState struct {
	Running  int  `json:"running"`
	Nodes    int  `json:"nodes"`
	Starting bool `json:"starting"`
}

// Build is one firmware image, as the library sees it. Snapshot.
//
// Version, board and role travel together because a board image is not a build
// on its own: "wadamesh" means nothing until it is wadamesh for a LilyGo_TDeck,
// built as a companion. A host build carries neither of the other two.
type Build struct {
	Role    string `json:"role"`
	Version string `json:"version"`
	Board   string `json:"board"`
	Bytes   int64  `json:"bytes"`
	OnDisk  bool   `json:"on_disk"`
	Path    string `json:"path"`
	InUse   int    `json:"in_use"`
	// Unavailable marks a build that exists only because nodes are pinned to
	// it: nothing on disk, nothing published. Pinning to one succeeds and then
	// fails at start, which reads as the library losing builds rather than as
	// a pin nobody can honour.
	Unavailable bool `json:"unavailable"`
}

// Describe is how this build is named where a person will read it.
func (b Build) Describe() string {
	if b.Board == "" {
		return b.Version
	}
	return b.Board + " - " + b.Role + " " + b.Version
}

// JobInfo is a long operation in flight. Snapshot; ask again for progress, or
// use Job.Wait.
type JobInfo struct {
	ID       string `json:"id"`
	What     string `json:"what"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Finished bool   `json:"finished"`
}

// Provenance is what a measurement was measured under.
//
// Carried with any result that is a number about the world, because a scripted
// number gets pasted into a report with the caveats stripped. The caveats have
// to be in the value.
type Provenance struct {
	// RFMode is "calculated" or "waveform".
	RFMode string `json:"rf_mode"`
	// ExcessLossDB is the calibration term in force, and Calibrated says it
	// was fitted against real receptions rather than left at the default.
	ExcessLossDB float64 `json:"excess_loss_db"`
	Calibrated   bool    `json:"calibrated"`
	Seed         uint64  `json:"seed"`
}

// String is one line, meant to be printed above any number a script emits.
func (p Provenance) String() string {
	fit := "default excess loss"
	if p.Calibrated {
		fit = "excess loss fitted to real receptions"
	}
	return "MeshBench: " + p.RFMode + " reception, " + fit +
		" — a best case; no multipath, no body loss, no oscillator error"
}
