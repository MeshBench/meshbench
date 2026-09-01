// The network as the panels see it: nodes, links, the ground they sit on and
// what each one has done.
package state

import (
	"image"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"
)

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
	// Board is the hardware that build is for, empty for a build made for
	// this machine. Kept beside the version because on its own a version is
	// not a build: "wadamesh" means nothing until it is wadamesh for a
	// LilyGo_TDeck.
	Board string
	// Hardware is what the node itself is, by board profile name.
	//
	// Not the same fact as Board, however often the two agree: Board is what
	// the *image* was built for, and this is what the node *is*. They come
	// apart the moment somebody points a host build at a node that is a
	// T-Deck, which is a perfectly ordinary thing to do, and reporting one as
	// the other would say the node had changed hardware when it had changed
	// firmware.
	Hardware string
	// CardSlot reports that this node's board has a card slot at all, and
	// CardFitted that there is a card in it. A slot is not a card: the board
	// says the slot exists, the node says whether it is filled.
	CardSlot   bool
	CardFitted bool
	// CardFile is the file behind that card, whether it is the node's own or
	// one it was handed, and CardShared says which. Reported rather than
	// guessed from the path: only the session knows where a node's own card
	// would be, and a panel comparing filenames gets it wrong for anybody who
	// pointed a node at a file that happens to be called card.img.
	CardFile   string
	CardShared bool
	// CardRequired says the firmware this node runs will not get far without
	// storage, so the slot is filled whatever the node would have preferred.
	// Shown rather than hidden, because a toggle that will not move needs to
	// say who is holding it.
	CardRequired bool

	// TrueRF marks a receiver that takes waveform verdicts whatever the
	// run's RF mode - the hybrid flag.
	TrueRF   bool
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
	// Antenna is what that pattern is, in the words somebody chose it in.
	//
	// Beside the samples rather than instead of them, because the two answer
	// different questions: the samples are the shape on the map, and this is
	// the setting a form fills in and a person changes. The samples cannot be
	// read back into a pattern - they are one-way, at the horizon, with the
	// feedline already folded in - so a panel that had only those could show an
	// antenna and never edit one.
	Antenna Antenna
}

// Antenna is a node's antenna as the interface needs it: which sort, and where
// it points.
//
// Flat strings and numbers rather than the model's own types, for the same
// reason Pattern is samples: the renderer draws a snapshot and does not know
// what a radiation pattern is. Type is empty for a node that has no antenna at
// all, which is said rather than drawn as an omni at 0 dBi.
type Antenna struct {
	Type          string
	GainDBiPeak   float64
	BeamwidthDeg  float64
	FrontToBackDB float64
	BearingDeg    float64
	DowntiltDeg   float64
	Polarisation  string
	FeedlineDB    float64
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
	// Radio is what this node's chip has actually been configured to be, as
	// the firmware left it. Not what the board profile claims it can do: the
	// two diverge whenever the firmware has a fault, and until this reached
	// here there was nowhere to see that they had.
	Radio RadioState

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

	// Board is which hardware this node is, empty for a node running a host
	// build. It is what decides the shape of the Hardware tab: a board's
	// screen, lamps and buttons are properties of the board, so the interface
	// asks the board rather than carrying a setting that could disagree
	// with it.
	Board string

	// Screen is the last picture this node's display sent, or nil where the
	// board has no display or has drawn nothing yet.
	//
	// Nil for both on purpose: which of the two it is comes from the board's
	// own declaration, not from here, and inventing an empty picture would
	// report a board with no screen and a board with a blank one as the same
	// thing.
	Screen *Screen
}

// Screen is one picture from a board's display.
//
// Bits rather than pixels, as the controller holds them: byte n carries eight
// vertical pixels of column n%Width in page n/Width. Kept in that form because
// converting here would lose the only property that makes it worth showing -
// that it is exactly what the firmware drew.
type Screen struct {
	Width, Height int
	// On is the display's own power state. MeshCore switches the panel off
	// after an idle, so a blank picture with On false is a sleeping board and
	// a blank one with On true is a board that cleared its screen. Drawing
	// them the same way reports the first as broken.
	On bool
	// BPP is one for a monochrome panel and sixteen for a colour one. Carried
	// rather than inferred from the size: a wrong guess draws something the
	// firmware did not send.
	BPP  int
	Bits []byte
}

// Lit reports whether a pixel is on, which only a monochrome panel can answer.
func (s *Screen) Lit(x, y int) bool {
	if s == nil || s.BPP != 1 || x < 0 || y < 0 || x >= s.Width || y >= s.Height {
		return false
	}
	i := (y/8)*s.Width + x
	if i >= len(s.Bits) {
		return false
	}
	return s.Bits[i]&(1<<(y%8)) != 0
}

// At is the colour of a pixel on a colour panel, as RGB565.
func (s *Screen) At(x, y int) (r, g, b uint8, ok bool) {
	if s == nil || s.BPP != 16 || y < 0 || y >= s.Height {
		return 0, 0, 0, false
	}
	return peripheral.RGB565At(s.Bits, s.Width, x, y)
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

// Area is one study boundary: outer rings, and holes that are outside it.
type Area struct {
	Name  string
	Rings [][]Point
	Holes [][]Point
}

// Point is a position, and the only geometry the snapshot carries.
type Point struct{ Lat, Lon float64 }

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
	// Worst is the sample that decides the verdict - where the first Fresnel
	// zone is most intruded on - so the chart can point at the cause rather
	// than leave it to be found by eye.
	Worst ProfileWorst
	// Assumed names the loss model the margins came from, for the panel to
	// say out loud. A margin whose provenance is silent reads as measured.
	Assumed string
}

// ProfileSample is one point along the path, in metres.
type ProfileSample struct {
	DistM float64
	// GroundM is the bare terrain, for the masts standing on their own
	// ground; BulgedM is the same ground with the earth's curvature in it,
	// which is what the chart fills.
	GroundM  float64
	BulgedM  float64
	LOSm     float64
	FresnelM float64
}

// ProfileWorst is where a path comes closest to failing, or fails.
type ProfileWorst struct {
	DistM      float64
	ClearanceM float64
	// FresnelPct is how much of the first Fresnel zone is clear there, in
	// percent; below 60 the link starts paying for the ground.
	FresnelPct float64
	Blocked    bool
}

// ProfileEdge is one obstruction and its own Bullington contribution -
// presented as a decomposition, never as an addition.
type ProfileEdge struct {
	DistM  float64
	LossDB float64
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

// RadioState is a node's chip as the firmware has set it up.
//
// Reported raw, and rendered raw: a register is worth showing as a register,
// because the question this answers is "is this node set to what I think it is",
// and a value translated on the way loses the ability to answer it.
type RadioState struct {
	// Reported says the node's radio has said anything at all. A node that has
	// not come up must not read as one configured to zero.
	Reported bool
	// GainReg is 0x08AC: 0x96 boosted, 0x94 power saving.
	GainReg    uint8
	Boosted    bool
	TxPowerDBm int8
	// FemLive is the front-end module's enable line now; FemAtTx is where it
	// stood when this node last began transmitting, which is the one that
	// decides how much power left the board.
	FemLive bool
	FemAtTx uint8
	// Mode is 0 standby, 1 rx, 2 tx, 3 cad.
	Mode         uint8
	SF, CR       uint8
	FreqHz       uint32
	BandwidthHz  uint32
	PreambleSyms uint16
	// IRQMask is what the firmware allowed to raise DIO1; IRQFlags is what is
	// raised now. The pair tells a node stuck on a flag from one with nothing
	// to say.
	IRQMask, IRQFlags uint16
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
	// Path is where this build sits on disk, empty when OnDisk is false. A
	// delete acts on this, not on role/version/board - those name the build,
	// this is where it actually lives.
	Path string
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
	// Native marks a build for this machine rather than an image for a board.
	// The two are not interchangeable and only one of them can be renamed.
	Native bool
	// Modified is when the file was last written, which is how a build
	// imported twice under one name is told from the one before it.
	Modified time.Time
	// Facts is what reading the front of the image says about it, and
	// Settings what has been decided about it. Both zero for a build that is
	// not on disk, where there is nothing to read and nothing decided.
	Facts    emulated.ImageFacts
	Settings firmware.BuildSettings
}

// OutputPane is one node's raw output from one source.
//
// A tail rather than the whole file, with Total saying what the file holds so
// a pane can report the difference rather than implying it has everything.
type OutputPane struct {
	Node   string
	Source string
	Lines  []string
	Total  int
	Path   string
	// Note is why this source is empty, when it is empty for a reason - a
	// board whose console is on USB has nothing on UART0 after the bootloader,
	// and a blank pane with no explanation reads as a broken one.
	Note string
	// Tracing is what the emulator was asked to trace, so a pane showing an
	// enormous log can say why it is enormous.
	Tracing string
}
