package scenario

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/sdr"
)

// Kind is what a node is.
//
// The distinction that matters is not what a node is called but whether it runs
// firmware. A repeater and a companion differ in configuration; an SDR observer
// differs in that there is no firmware to configure, so most of the machinery
// around nodes simply does not apply to it.
type Kind string

const (
	// SimpleRepeater forwards; AdvancedRepeater also serves clients and holds
	// state. Both run MeshCore.
	SimpleRepeater   Kind = "simple-repeater"
	AdvancedRepeater Kind = "advanced-repeater"

	// Companion is a user's device — the thing a phone connects to.
	Companion Kind = "companion"

	// RoomServer holds posts for clients to collect, and does not forward. It
	// is a repeater in every way that touches this simulator — same console,
	// same admin password, same place in a scenario — except the one that
	// matters on the air: traffic it hears does not go back out. A mesh that
	// treats it as a repeater will overstate its own reach.
	RoomServer Kind = "room-server"

	// SDRObserver runs no firmware and transmits nothing. It captures the
	// summed field at its antenna and hands back IQ: a waterfall, a recording,
	// and nothing that has been decided.
	//
	// It exists because every other node type eventually makes a judgement —
	// this decoded, that collided — and a simulator whose only outputs are its
	// own judgements cannot be checked against itself. An observer is the
	// instrument you point at the simulation.
	SDRObserver Kind = "sdr-observer"

	// Emitter is an external interference source, per ADR-0012: a mast
	// carrying something that is not MeshCore — broadcast, paging, PMR — and
	// still shaping every nearby receiver's noise floor. It is propagated
	// through the same terrain engine as everything else, so a mast behind a
	// hill interferes less, which is the whole point of doing it properly.
	Emitter Kind = "emitter"
)

// Role is the MeshCore application a node runs, named as upstream names its
// example directory.
//
// A named type because it is the string every firmware verb is keyed on -
// pinning a build, asking what is needed, importing your own - and the
// published catalogue spells the same things differently ("repeater",
// "room-server"). Those shorter names belong to the release assets and are
// normalised away on the way in; a caller who types one at a verb gets a role
// nothing matches and a mesh with no firmware, which reports as nothing at all
// until the run refuses to start.
type Role string

// The roles. The transport variants exist only for board images: a board
// publishes both companion builds at one version, and which one a node runs is
// not something to leave to whichever was downloaded last.
const (
	RoleSimpleRepeater    Role = "simple_repeater"
	RoleCompanionRadio    Role = "companion_radio"
	RoleSimpleRoomServer  Role = "simple_room_server"
	RoleCompanionRadioUSB Role = "companion_radio_usb"
	RoleCompanionRadioBLE Role = "companion_radio_ble"
)

// Application is the MeshCore application a node of this kind runs.
//
// A default, not a definition. What a node actually is depends on which firmware
// is loaded onto it, and Node.Firmware overrides this — so a node type MeshCore
// ships next year needs no new Kind, only its application name.
func (k Kind) Application() Role {
	switch k {
	case Companion:
		return RoleCompanionRadio
	case RoomServer:
		return RoleSimpleRoomServer
	case SDRObserver, Emitter:
		return "" // no firmware to run
	default:
		// Both repeater kinds run the same application; they differ in how it is
		// configured, which is the firmware's business and not ours.
		return RoleSimpleRepeater
	}
}

// Transmits reports whether the node is a mesh transmitter — a possible end
// of a link, a coverage source, a provisioning target. An Emitter radiates
// but does none of those; its power enters the model as noise, not as frames.
func (k Kind) Transmits() bool { return k != SDRObserver && k != Emitter }

// CardSlot is what is in a board's card slot.
type CardSlot string

const (
	// CardAsBoard is the board's own answer: a card in every slot it
	// declares. The zero value, so a scenario saved before any of this loads
	// the way it always did.
	CardAsBoard CardSlot = ""
	// CardFitted and CardEmpty are a decision about this node, which is what
	// makes them worth saving: "this handheld has no card in it" is part of
	// the scenario, not a detail of the run.
	CardFitted CardSlot = "fitted"
	CardEmpty  CardSlot = "empty"
)

// HasCard reports whether this node's slot holds a card, given whether its
// board has a slot at all and whether its firmware insists on one.
//
// A firmware that requires storage wins, because a build that will not boot
// without a card is not something a per-node preference should be able to
// half-configure into failing several minutes later.
func (n Node) HasCard(boardHasSlot, firmwareRequires bool) bool {
	if !boardHasSlot {
		return false
	}
	if firmwareRequires {
		return true
	}
	return n.Card != CardEmpty
}

// RunsFirmware reports whether a node needs a firmware backend.
func (k Kind) RunsFirmware() bool { return k != SDRObserver && k != Emitter }

// FirmwareRef names a firmware build.
//
// Version is an upstream ref — a tag, or "main" or "dev" — which makes "does
// this behave differently on the development branch" a question the workbench
// can answer by changing a string.
type FirmwareRef struct {
	Role    Role
	Version string

	// Board names the hardware this node emulates, and empty means the host
	// build. It is what decides which of the two backends runs the node, so it
	// belongs on the node rather than being inferred later: a scenario that
	// mixes emulated and native nodes is the point of having both, and a reader
	// must never have to guess which a node is.
	Board string
}

// Emulated reports whether this node runs published hardware firmware rather
// than a build for this machine.
func (f FirmwareRef) Emulated() bool { return f.Board != "" }

// Node is one placed thing in a scenario.
type Node struct {
	// Name is how it appears in the ledger, in captures and in the UI. Unique
	// within a scenario.
	Name string
	Kind Kind

	// Board is the hardware this node is, by profile name. It decides the
	// transmit ceiling, the receive chain's noise figure, and - the reason it
	// had to become a stored field rather than a placement-time default - the
	// battery and panel the energy model needs. Empty means unknown, and the
	// energy model says so instead of inventing a pack.
	Board string

	// PublicKey is the *real* node's key, kept from the import as an external
	// reference. It is what a merge joins on, and what matches observed
	// traffic back to this node.
	//
	// It is emphatically not this node's identity in the simulation: the
	// firmware generates its own keypair at first boot, because nobody has
	// the real private key. Anything comparing a running node's key to this
	// one will never match.
	PublicKey string

	// Regions are the MeshCore transport regions this node holds, and
	// DefaultScope the one it scopes its own traffic to. Observed from real
	// traffic rather than declared: a repeater's self-reported default scope is
	// empty for most of them, while what it relays is not optional.
	//
	// Carried on the node so a scenario built from a live deployment starts its
	// firmware configured the way the real one is.
	Regions      []string
	DefaultScope string

	// AllowAnyFlood makes this node forward flood traffic whatever region it
	// is scoped to, by allowing the wildcard region rather than only the ones
	// it holds. `region allowf *`.
	//
	// It is deliberately a stored field and not a console command typed at a
	// running node: what the firmware was told at boot does not survive into a
	// saved scenario, so a fixture claiming to be permissive would load as a
	// strict one and nothing would say so. It is more permissive than any real
	// network, which is why it is per node and off unless asked for.
	AllowAnyFlood bool

	// EmitterDutyPct is how much of the time an Emitter is keyed, 0-100.
	// Duty matters as much as power: a paging transmitter at 10% is a
	// different neighbour from a broadcast carrier at 100%.
	EmitterDutyPct float64

	// FloodMaxSeen is the largest hop count this node was observed relaying — a
	// lower bound on its flood.max, never the value itself.
	FloodMaxSeen int

	// Firmware is the application and MeshCore version to run, overriding what
	// Kind would pick. Empty fields fall back to Kind.Application() and to the
	// newest published build.
	Firmware FirmwareRef

	// Position, and the uncertainty it was imported with. A CoreScope record
	// at +/-5 km does not get a confident answer, so the uncertainty travels
	// with the node rather than being dropped at the import boundary.
	Position      LatLon
	UncertaintyKm float64

	// HeightAGLm is antenna height above ground. Ground elevation comes from
	// the DEM and is never typed — the operator knows how tall their mast is,
	// not what height AMSL its top is at.
	HeightAGLm float64

	Antenna antenna.Mounted

	// Radio is the LoRa configuration. An observer uses CentreHz and the
	// bandwidth to decide what it is looking at; everything else ignores SF
	// and coding rate for an observer, because it never modulates anything.
	Radio RadioConfig

	// TxPowerDBm is what it transmits at. Zero for an observer, and validated
	// as such rather than silently ignored.
	TxPowerDBm float64

	// NoiseFigureDB is the receiver's own noise contribution. Set per node
	// because a repeater with a masthead preamp and a handheld in a pocket do
	// not have the same one.
	NoiseFigureDB float64
	// TrueRF asks for waveform verdicts at this receiver even when the run
	// is in calculated mode - the hybrid the waveform plan describes: a big
	// mesh priced fast, with full-fidelity reception where it matters.
	TrueRF bool `json:"true_rf,omitempty"`

	// Card is what is in this node's card slot, on a board that has one.
	//
	// Carried on the node rather than decided by the board, because a slot is
	// not a fitted card: two of the same handheld in one scenario, one with
	// storage and one without, is an ordinary thing to want and the board can
	// only say that the slot exists. Empty means the board's own answer, which
	// is a card in every slot a board declares - the behaviour before this
	// existed, so a saved scenario loads unchanged.
	Card CardSlot `json:"card,omitempty"`

	// CardFile is the file behind that card, or empty for the node's own,
	// named after it and kept beside its flash. Set it to share one card
	// between runs, or to hand a node a card somebody else prepared.
	CardFile string `json:"card_file,omitempty"`

	// FEM is the front-end module this node's board carries, where it has one.
	// Nil means the radio drives the antenna directly, and then whether the
	// firmware drives a transmit-enable line is not a question worth asking.
	FEM *FEM `json:",omitempty"`
}

// RadioConfig is the modem setup.
type RadioConfig struct {
	CentreHz     float64
	BandwidthHz  float64
	SpreadFactor int
	CodingRate   int // 1..4 for 4/5..4/8
}

// Validate reports what is wrong with a node, in terms of the thing the user
// set rather than the field that failed.
//
// Deliberately strict about an observer's transmit power. Silently ignoring it
// would let someone place a "listening" node that they believe is transmitting,
// or the reverse, and nothing downstream would look unusual.
func (n Node) Validate() error {
	if n.Name == "" {
		return fmt.Errorf("scenario: a node needs a name")
	}
	switch n.Kind {
	case SimpleRepeater, AdvancedRepeater, Companion, RoomServer, SDRObserver, Emitter:
	default:
		return fmt.Errorf("scenario: %s: unknown kind %q", n.Name, n.Kind)
	}
	if n.HeightAGLm < 0 {
		return fmt.Errorf("scenario: %s: antenna height %.1f m is below ground", n.Name, n.HeightAGLm)
	}
	if n.Antenna.Pattern == nil {
		return fmt.Errorf("scenario: %s: no antenna pattern", n.Name)
	}
	if n.Radio.CentreHz <= 0 || n.Radio.BandwidthHz <= 0 {
		return fmt.Errorf("scenario: %s: needs a centre frequency and a bandwidth", n.Name)
	}

	if n.Kind == Emitter {
		if n.TxPowerDBm <= 0 {
			return fmt.Errorf("scenario: %s: an emitter with no power emits nothing", n.Name)
		}
		if n.EmitterDutyPct < 0 || n.EmitterDutyPct > 100 {
			return fmt.Errorf("scenario: %s: duty cycle %.0f%% is not 0-100", n.Name, n.EmitterDutyPct)
		}
		return nil
	}
	if !n.Kind.Transmits() {
		if n.TxPowerDBm != 0 {
			return fmt.Errorf("scenario: %s: an SDR observer transmits nothing, but is set to %.1f dBm",
				n.Name, n.TxPowerDBm)
		}
		return nil
	}

	if n.Radio.SpreadFactor < 5 || n.Radio.SpreadFactor > 12 {
		return fmt.Errorf("scenario: %s: spreading factor %d is outside SF5-SF12",
			n.Name, n.Radio.SpreadFactor)
	}
	if n.Radio.CodingRate < 1 || n.Radio.CodingRate > 4 {
		return fmt.Errorf("scenario: %s: coding rate %d is not 1..4 (4/5 to 4/8)",
			n.Name, n.Radio.CodingRate)
	}
	if n.TxPowerDBm <= 0 {
		return fmt.Errorf("scenario: %s: transmits, but at %.1f dBm", n.Name, n.TxPowerDBm)
	}
	return nil
}

// Observer builds the receive-only instrument this node describes.
//
// Only an SDR observer has one. Asking any other kind for it is a programming
// mistake rather than a user one, so it returns an error rather than a
// zero-valued observer that would silently capture nothing.
func (n Node) Observer() (sdr.Observer, error) {
	if n.Kind != SDRObserver {
		return sdr.Observer{}, fmt.Errorf("scenario: %s is a %s, not an SDR observer", n.Name, n.Kind)
	}
	if err := n.Validate(); err != nil {
		return sdr.Observer{}, err
	}
	return sdr.Observer{
		Name:     n.Name,
		CentreHz: n.Radio.CentreHz,
		// The observer sees exactly the modelled channel — see
		// docs/shortcomings.md §1.6 on why that is narrower than a real dongle.
		SampleRateHz:  n.Radio.BandwidthHz,
		NoiseFigureDB: n.NoiseFigureDB,
	}, nil
}
