package firmware

// RadioStats is what the node's radio reports about the channel decisions the
// firmware made through it.
type RadioStats struct {
	// IRQReads is how many times the driver read the interrupt register, and
	// BusyReads how many of those found a detection flag set - the firmware's
	// own view of how often the air looked occupied.
	IRQReads, BusyReads uint32
	// BusyMs is how long those flags were up.
	BusyMs uint32
	// SpuriousUp counts deliberately injected false detections, on the faulty
	// chip variants. Zero on a chip that behaves.
	SpuriousUp uint32

	// Configured reports that the fields below were sent at all. A node built
	// before the radio reported its own configuration leaves them zero, and
	// zero is a legal value for most of them.
	Configured bool

	// RxGainReg is register 0x08AC as the firmware last left it: 0x96 boosted,
	// 0x94 power saving. Reported raw because deciding what a value is worth in
	// decibels is the engine's business, not the chip's.
	//
	// Worth watching because MeshCore re-applies the compile-time
	// SX126X_RX_BOOSTED_GAIN macro on every AGC reset rather than the operator's
	// runtime setting, so the two silently diverge - and the firmware's own CLI
	// keeps reporting the setting the operator chose.
	RxGainReg uint8

	// TxPowerDBm is what SetTxParams asked the PA for. Not what leaves the
	// antenna: a board with a front-end module adds to this, and only if the
	// firmware switched the module on.
	TxPowerDBm int8

	// FemEnabled is the front-end module's transmit-enable line as it stands
	// now. Useful for showing what the radio is doing; not the right thing to
	// compute power from, because the line is meant to be low while the node
	// listens and RadioLib only raises it just before a transmission.
	FemEnabled bool

	// FemAtTx is where the line stood at the instant this node last began
	// transmitting, which is the only moment it decides anything.
	FemAtTx FemState

	// The modem, as the firmware programmed it. None of this was visible from
	// outside the chip before, which is how a node set to the wrong spreading
	// factor looks exactly like one set to the right one.
	Mode         uint8 // 0 standby, 1 rx, 2 tx, 3 cad
	SF           uint8
	CR           uint8
	FreqHz       uint32
	BandwidthHz  uint32
	PreambleSyms uint16

	// IRQMask is what the firmware allowed to raise DIO1; IRQFlags is what is
	// currently raised. The pair is how a node that has stopped transmitting
	// because a flag stuck is told apart from one with nothing to say.
	IRQMask, IRQFlags uint16
}

// FemState is whether a board's front-end module was in circuit when the node
// transmitted. Three states rather than a bool: a node that has not transmitted
// has not answered the question, and reading that as "the module was out" would
// dock it for having nothing to say.
type FemState uint8

const (
	FemUnknown FemState = 0
	FemOut     FemState = 1
	FemIn      FemState = 2
)

// Receive gain, as RadioLib writes it to register 0x08AC.
const (
	RxGainBoosted     uint8 = 0x96
	RxGainPowerSaving uint8 = 0x94
)

// RxBoosted reports whether the chip is in boosted receive gain.
//
// Only meaningful when Configured: an unreported register reads as zero, which
// is neither of the two values the datasheet defines.
func (s RadioStats) RxBoosted() bool { return s.RxGainReg == RxGainBoosted }

// Stats reports the radio's account of this node.
func (b *Bridge) Stats() RadioStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stats
}
