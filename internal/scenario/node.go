package scenario

import (
	"fmt"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/sdr"
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

	// SDRObserver runs no firmware and transmits nothing. It captures the
	// summed field at its antenna and hands back IQ: a waterfall, a recording,
	// and nothing that has been decided.
	//
	// It exists because every other node type eventually makes a judgement —
	// this decoded, that collided — and a simulator whose only outputs are its
	// own judgements cannot be checked against itself. An observer is the
	// instrument you point at the simulation.
	SDRObserver Kind = "sdr-observer"
)

// Transmits reports whether a node ever puts energy on the channel.
//
// Worth a method rather than a comparison at each call site: an observer that
// is accidentally treated as a transmitter contributes a phantom signal to
// every other node's reception, and the result still looks like a plausible
// mesh.
func (k Kind) Transmits() bool { return k != SDRObserver }

// RunsFirmware reports whether a node needs a firmware backend.
func (k Kind) RunsFirmware() bool { return k != SDRObserver }

// Node is one placed thing in a scenario.
type Node struct {
	// Name is how it appears in the ledger, in captures and in the UI. Unique
	// within a scenario.
	Name string
	Kind Kind

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
	case SimpleRepeater, AdvancedRepeater, Companion, SDRObserver:
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
