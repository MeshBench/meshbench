// The values a caller holds.
//
// Snapshots, all of them: a value read once, which never changes afterwards.
// The live things - the workbench, a node handle - are types with methods, and
// they re-read the session every time they are asked. Which one a thing is
// decides whether it can be held across a Run and still be true, so it is
// stated on each.
package meshbench

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
	// Project and Nodes are what this session has open, which is what tells
	// two of them apart in a list.
	Project string `json:"project"`
	Nodes   int    `json:"nodes"`
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
	Kind     Kind     `json:"kind"`
	Lat      float64  `json:"lat"`
	Lon      float64  `json:"lon"`
	HeightM  float64  `json:"height_m"`
	TxDBm    float64  `json:"tx_dbm"`
	Regions  []string `json:"regions"`
	Firmware string   `json:"firmware"`
	// Board is what the node is; FirmwareBoard is what its image was built
	// for. They agree most of the time and come apart the moment a host build
	// is pointed at a T-Deck, which is an ordinary thing to do.
	Board         Board  `json:"board"`
	FirmwareBoard string `json:"firmware_board"`
	Sent          int    `json:"sent"`
	Heard         int    `json:"heard"`
	Selected      bool   `json:"selected"`
}

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
	Class  Class    `json:"class"`
}

// SimState is the clock. Snapshot.
type SimState struct {
	Playing bool   `json:"playing"`
	NowMs   uint32 `json:"now_ms"`
	UntilMs uint32 `json:"until_ms"`
	Events  int    `json:"events"`
	StepMs  uint32 `json:"step_ms"`
	Seed    uint64 `json:"seed"`
	// Warming, LinksMeasured and WarmHeld are the three states of the link
	// measurement, and they are three because a warm that stopped to ask
	// permission to download terrain is neither running nor finished. Reading
	// "not warming" as "measured" is how a script came to read a study over
	// ground the workbench never fetched.
	Warming       bool `json:"warming"`
	LinksMeasured bool `json:"links_measured"`
	WarmHeld      bool `json:"warm_held"`
	// Ground is what elevation data the studies here have under them.
	Ground Ground `json:"ground"`
}

// FirmwareState is how far a start has got. Snapshot.
type FirmwareState struct {
	Running int `json:"running"`
	// Nodes is the nodes that run firmware, which is not every node: an SDR
	// observer and an emitter never boot one. Comparing Running against the
	// scenario's size is how a wait ends up asking for 58 of 58 on a mesh
	// where only 56 can ever start.
	Nodes    int  `json:"nodes"`
	Total    int  `json:"total"`
	Starting bool `json:"starting"`
}

// Build is one firmware image, as the library sees it. Snapshot.
//
// Version, board and role travel together because a board image is not a build
// on its own: "wadamesh" means nothing until it is wadamesh for a LilyGo_TDeck,
// built as a companion. A host build carries neither of the other two.
type Build struct {
	Role    Role   `json:"role"`
	Version string `json:"version"`
	Board   Board  `json:"board"`
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

// BuildDetails is one build in full: what a library row cannot hold.
//
// Separate from Build because the library is deliberately a list - role,
// version, size, a tick. Where the file actually is, whether it is a whole
// flash image or half of one, and what has been decided about how it runs are
// the questions somebody has once a build does not do what they expected.
type BuildDetails struct {
	Role    Role   `json:"role"`
	Version string `json:"version"`
	Board   Board  `json:"board"`
	// Native marks a build for this machine rather than an image for a board.
	// The two are not interchangeable and only one of them can be renamed.
	Native bool   `json:"native"`
	OnDisk bool   `json:"on_disk"`
	Path   string `json:"path"`
	// SettingsPath is where the settings below are written, named whether or
	// not any exist: "where does this live" is asked of a build that has none
	// as often as of one that has.
	SettingsPath string `json:"settings_path"`
	Bytes        int64  `json:"bytes"`
	Modified     string `json:"modified"`
	InUse        int    `json:"in_use"`
	// Kind is what reading the front of the image says it is, and Bootable
	// whether a board could start from it. An application-only image imports,
	// lists and pins exactly like a whole one and then starts nothing.
	Kind     string `json:"kind"`
	Bootable bool   `json:"bootable"`
	FlashMB  int    `json:"flash_mb"`
	// CoprocAtReset, CardRequired and Notes are kept beside the image, so
	// they follow this build rather than the board it runs on.
	CoprocAtReset bool `json:"coproc_at_reset"`
	// CardRequired says this firmware will not get far without storage in the
	// board's slot, so every node running it is given a card whatever its own
	// slot was set to.
	CardRequired bool   `json:"card_required"`
	Notes        string `json:"notes"`
}

// Describe is how this build is named where a person will read it.
func (b BuildDetails) Describe() string {
	if b.Board == "" {
		return b.Version
	}
	return string(b.Board) + " - " + string(b.Role) + " " + b.Version
}

// Describe is how this build is named where a person will read it.
func (b Build) Describe() string {
	if b.Board == "" {
		return b.Version
	}
	return string(b.Board) + " - " + string(b.Role) + " " + b.Version
}

// JobInfo is a long operation in flight. Snapshot; ask again for progress, or
// use Job.Wait.
type JobInfo struct {
	ID       string `json:"id"`
	What     string `json:"what"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Finished bool   `json:"finished"`
	// Failed marks a job that ended without doing what it was for. Separate
	// from Finished because a waiter needs both: "stop waiting" and "this did
	// not work" are different answers, and telling them apart by reading What
	// means matching on prose.
	Failed bool `json:"failed"`
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

// NameMatch is one answer from a name search, and how sure it is.
//
// Score runs 0 to 1, ranked best first by the workbench. It exists so a script
// can tell "found it" from "found something that shares a word": a top result
// at 0.3 is a prompt to look at the list, not a node to start talking to.
type NameMatch struct {
	Name     string  `json:"name"`
	Score    float64 `json:"score"`
	Kind     Kind    `json:"kind"`
	Lat, Lon float64 `json:"-"`
}

func (m NameMatch) String() string { return m.Name }

// Neighbour is one node near another, with how far away it is.
type Neighbour struct {
	Name     string  `json:"name"`
	Km       float64 `json:"km"`
	Kind     Kind    `json:"kind"`
	Lat, Lon float64 `json:"-"`
}

func (n Neighbour) String() string { return n.Name }

// CardSlot is what is in one node's card slot.
//
// A slot is not a fitted card: the board says the slot exists, the node says
// whether it is filled, and a firmware that keeps its settings on a card fills
// it regardless.
type CardSlot struct {
	Node string `json:"node"`
	// Slot is "" for the board's own answer, "fitted" or "empty" for a
	// decision somebody made about this node.
	Slot   string `json:"slot"`
	Fitted bool   `json:"fitted"`
	// File is the card this node uses, and OwnFile the one it would use if it
	// had been handed none.
	File    string `json:"file"`
	OwnFile string `json:"own_file"`
	Bytes   int64  `json:"bytes"`
	// RequiredByFirmware says the build fills the slot whatever the node
	// asked for; BoardHasSlot that there is a slot at all.
	RequiredByFirmware bool `json:"required_by_firmware"`
	BoardHasSlot       bool `json:"board_has_slot"`
	Wiped              bool `json:"wiped"`
}

// JournalEntry is one command the workbench was driven with: its sequence, the
// wall-clock time it ran, the verb, and a compact rendering of its argument.
type JournalEntry struct {
	Seq   uint64 `json:"seq"`
	AtMs  int64  `json:"at_ms"`
	Verb  string `json:"verb"`
	Nodes int    `json:"nodes"`
	Err   string `json:"err,omitempty"`
	Arg   string `json:"arg,omitempty"`
}

// Journal is the command history: when the process started, and the commands
// since, newest last. Polls and the workers' own progress reports are left out,
// so this is how the world got here, not everything that touched the socket.
type Journal struct {
	StartedMs int64          `json:"started_ms"`
	Count     int            `json:"count"`
	Entries   []JournalEntry `json:"entries"`
}
