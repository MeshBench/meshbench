// A node's radio, as the firmware left it.
//
// Split from network.go, which describes itself as the network the panels see -
// nodes, links and the ground they sit on. A chip's registers are not that:
// they are one node's hardware, and the file was over its length limit with
// both in it.
package state

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
	GainReg uint8
	Boosted bool
	// ConsoleBaud is the rate the firmware set the board's console UART to, or
	// zero where nothing said. Zero has two causes and they are different
	// facts: a board whose console is the USB peripheral has no line rate at
	// all, and an emulator older than the field reports none - which of the
	// two it is comes from the board's own profile.
	ConsoleBaud uint32

	// TxPowerDBm is what SetTxParams asked the PA for, and TxPowerUnset when
	// the firmware has not called it at all. Ask through TxPowerSet rather
	// than comparing: the board view compared it as an ordinary number, found
	// -128 to be under the board's ceiling, and reported a radio that had
	// never been configured as agreeing with its profile.
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
	// IRQMask is what the firmware allowed into the chip's interrupt status
	// register; IRQFlags is what is raised now. The pair tells a node stuck on a
	// flag from one with nothing to say.
	IRQMask, IRQFlags uint16
	// DIO1Mask is the narrower set wired out to the DIO1 pin, which is a
	// different field of SetDioIrqParams and not the same thing as IRQMask.
	// Worth its own row because confusing the two is a fault that has happened:
	// the chip model gated the pin on the enable mask, so HeaderValid raised
	// DIO1 part-way through a carrier and the pin was still high when RxDone
	// arrived - no rising edge for a driver that attaches on one, and a board
	// that heard every advert and forwarded about one in three.
	//
	// RadioLib's receive default is RxDone alone, against an IRQMask that also
	// carries Timeout, CrcErr, HeaderValid and HeaderErr, so the two being equal
	// is a sign rather than a normal reading.
	DIO1Mask uint16
	// DIO1Reported tells "this node did not say" from "this node said zero", the
	// same way Reported does for the block above: a radioserver older than this
	// field sends a shorter record.
	DIO1Reported bool
}

// TxPowerUnset is what TxPowerDBm holds when the firmware has never called
// SetTxParams. Not a power: the sentinel an int8 has room for below anything a
// PA can be asked for.
const TxPowerUnset int8 = -128

// TxPowerSet reports whether the firmware has asked the PA for anything.
//
// A method rather than a comparison at each caller, because the two windows
// that draw this had one each and only one of them was right: the board view
// found -128 to be under the board's 22 dBm ceiling and reported a radio that
// had never been configured as agreeing with its own profile - a false
// "agrees" in the column that exists to be trusted, while somebody was using
// it to work out why nothing was transmitting.
func (r RadioState) TxPowerSet() bool { return r.TxPowerDBm != TxPowerUnset }
