// Package firmware connects emulated nodes to the RF engine.
//
// An emulated node runs inside Renode or QEMU and reaches the simulator over a
// socket: the SX1262 peripheral model (tools/renode/peripherals/SX1262.cs)
// hands transmitted frames out and takes delivered frames back.
//
// A socket rather than an emulator-native wireless medium is deliberate. The
// physics lives in internal/rf, and an emulated node must share exactly the
// same channel as a native one — otherwise the two backends are not comparable,
// which is the entire reason ADR-0010 has both.
package firmware

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// Message kinds on the wire.
const (
	kindFrame  = 0x01 // a radio frame, either direction
	kindTick   = 0x02 // host → node: advance the node's clock to this sim time
	kindAck    = 0x03 // node → host: everything up to that tick has been processed
	kindTxDone = 0x04 // host → node: the waveform you started has finished on the air
	// kindOriginate asks the firmware to send a message of its own. Handing a
	// node a fabricated frame instead makes it a receiver of something it
	// cannot parse: MeshCore drops what is not a valid packet, correctly, and
	// nothing relays. Only the firmware can build a packet the rest of the
	// firmware will accept.
	kindOriginate = 0x05

	// The node's serial port, both directions.
	//
	// A real UART, not a rendering of one. MeshCore's applications carry their
	// own command interface on Serial — that is how a repeater is configured on
	// hardware — so a workbench that reaches it can administer a mesh rather than
	// only build one. Anything else would be a prompt that agrees with whatever
	// the simulator already believed.
	kindConsoleIn  = 0x06 // host → node: bytes typed at the node's UART
	kindConsoleOut = 0x07 // node → host: bytes the node printed

	// kindRadioStats is node → host: what the radio has been asked and what it
	// answered. Counting the firmware's own view of the channel is the only way
	// to tell "the mesh is busy" from "our virtual chip says so too readily",
	// and the second would look exactly like a finding about the firmware.
	kindRadioStats = 0x09

	// kindChannelBusy is host → node: is another station on the air here?
	//
	// The input to MeshCore's listen-before-talk. The node's radio cannot know
	// it - only the engine has every waveform in flight and how strongly each
	// one arrives at a given antenna.
	kindChannelBusy = 0x08
)

// ErrClosed is returned once a bridge has been shut down.
var ErrClosed = errors.New("firmware: bridge closed")

// Bridge is one emulated node's link to the RF engine.
//
// Wire format, both directions: [kind:1][length:2 big-endian][payload].
// Length-prefixed for the same reason the companion transports are — a stream
// gives no message boundaries, and a radio frame is a message.
type Bridge struct {
	console io.Writer
	// claimed marks the port as owned by a companion link, and claimGen
	// counts claims so a release can tell whether its own is still current.
	claimed  bool
	claimGen uint64

	ln   net.Listener
	node string

	mu     sync.Mutex
	conn   net.Conn
	closed bool

	// Transmitted carries frames the emulated firmware sent. The RF engine
	// reads these and puts them on the channel.
	Transmitted chan []byte

	// acked carries tick acknowledgements. Buffered by one: a node that is
	// behaving sends exactly one ack per tick, and a node that sends more is
	// broken in a way Advance should surface rather than absorb.
	acked chan uint32

	// stats is the radio's own account of what it was asked.
	stats RadioStats

	// done is closed when the bridge is. Anything waiting on the node must
	// select on it: a node whose process has gone will never ack, and a wait
	// with no way to hear about that hangs for as long as its deadline allows -
	// or for ever, if it was given none.
	done chan struct{}
}

// Listen starts a bridge for one emulated node. Addr is typically
// "127.0.0.1:0"; the emulator connects to Addr().
func Listen(addr, node string) (*Bridge, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("firmware: bridge listen: %w", err)
	}
	b := &Bridge{
		ln:          ln,
		node:        node,
		Transmitted: make(chan []byte, 32),
		acked:       make(chan uint32, 1),
		done:        make(chan struct{}),
	}
	go b.accept()
	return b, nil
}

// Addr is the address the emulator should connect to.
func (b *Bridge) Addr() string { return b.ln.Addr().String() }

// Node names the simulated node this bridge belongs to, for the ledger.
func (b *Bridge) Node() string { return b.node }

func (b *Bridge) accept() {
	for {
		c, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		// One emulated radio per bridge: a second connection would mean two
		// chips claiming one node, which is a wiring mistake worth refusing
		// rather than silently interleaving.
		if b.conn != nil {
			b.mu.Unlock()
			_ = c.Close()
			continue
		}
		b.conn = c
		b.mu.Unlock()
		go b.read(c)
	}
}

func (b *Bridge) read(c net.Conn) {
	defer func() {
		b.mu.Lock()
		if b.conn == c {
			b.conn = nil
		}
		// The one connection this bridge will ever have has just ended,
		// unasked - Close would already have set closed, so this only fires
		// when the node's process crashed or was killed outside the
		// graceful path. WaitAdvance blocks on b.done, and until now only
		// Close ever signalled it: a node that died mid-run left every
		// caller waiting on it blocked forever, and every node runFirmware
		// had not reached yet in that same tick frozen behind it, on every
		// tick after, silently. Treating a dropped connection as done here
		// is the same fact Close already expresses, just noticed the other
		// way round.
		wasOpen := !b.closed
		if wasOpen {
			b.closed = true
		}
		b.mu.Unlock()
		if wasOpen {
			close(b.done)
		}
		_ = c.Close()
	}()
	var hdr [3]byte
	for {
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(hdr[1:])
		buf := make([]byte, n)
		if n > 0 {
			if _, err := io.ReadFull(c, buf); err != nil {
				return
			}
		}
		switch hdr[0] {
		case kindFrame:
			if n == 0 {
				continue
			}
			select {
			case b.Transmitted <- buf:
			default:
				// The RF engine is not keeping up. Dropping here would look exactly
				// like a propagation result, so it must never be silent.
			}
		case kindConsoleOut:
			b.mu.Lock()
			w := b.console
			b.mu.Unlock()
			if w != nil && n > 0 {
				// Best effort. A console that cannot be written to must not stall
				// the node it belongs to: the simulation's correctness does not
				// depend on anyone reading the output.
				_, _ = w.Write(buf)
			}

		case kindRadioStats:
			if n >= 16 {
				st := RadioStats{
					IRQReads:   binary.BigEndian.Uint32(buf[0:]),
					BusyReads:  binary.BigEndian.Uint32(buf[4:]),
					BusyMs:     binary.BigEndian.Uint32(buf[8:]),
					SpuriousUp: binary.BigEndian.Uint32(buf[12:]),
				}
				// The configured-radio tail is newer than the counters, so it is
				// read on length rather than assumed. Configured is what tells
				// the engine "this node did not say" apart from "this node said
				// zero" - and the difference matters, because a zero transmit
				// power is a legal thing for a radio to be set to.
				if n >= 36 {
					st.RxGainReg = buf[16]
					st.TxPowerDBm = int8(buf[17])
					st.FemEnabled = buf[18] != 0
					st.Mode = buf[19]
					st.SF = buf[20]
					st.CR = buf[21]
					st.FreqHz = binary.BigEndian.Uint32(buf[22:])
					st.BandwidthHz = binary.BigEndian.Uint32(buf[26:])
					st.PreambleSyms = binary.BigEndian.Uint16(buf[30:])
					st.IRQMask = binary.BigEndian.Uint16(buf[32:])
					st.IRQFlags = binary.BigEndian.Uint16(buf[34:])
					st.Configured = true
				}
				if n >= 37 {
					st.FemAtTx = FemState(buf[36])
				}
				b.mu.Lock()
				b.stats = st
				b.mu.Unlock()
			}

		case kindAck:
			if n != 4 {
				return // malformed; the stream is not ours
			}
			select {
			case b.acked <- binary.BigEndian.Uint32(buf):
			default:
				return // an unsolicited ack means the node lost lockstep
			}
		default:
			return // desynchronised; an unknown kind means the stream is not ours
		}
	}
}

// Console directs the node's serial output at w, and reports whether anything
// was listening before.
//
// Set rather than subscribed: a node has one serial port, and two readers of one
// UART would each see half the output — which is worse than not having it.
func (b *Bridge) Console(w io.Writer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// A claimed port belongs to whoever claimed it. Without this the workbench
	// console re-attached itself on the very next frame and quietly took the
	// UART back from an attached companion client — which looks exactly like a
	// client that connected and then received nothing.
	if b.claimed {
		return
	}
	b.console = w
}

// Claim gives one owner exclusive use of the serial port, as a USB cable does.
//
// Returns a release function. While a claim is held, Console is ignored: two
// protocols interleaved on one UART is neither of them.
//
// The release only releases its own claim, once. An unconditional release
// let a stale holder clear whoever had claimed since - disconnect racing
// serve unplugged the TCP client that had just taken the port, which read as
// a client that connected and then received nothing. A generation number
// rather than comparing writers, because the same writer claiming again
// would make its old release match again.
func (b *Bridge) Claim(w io.Writer) func() {
	b.mu.Lock()
	b.claimGen++
	gen := b.claimGen
	b.console, b.claimed = w, true
	b.mu.Unlock()
	return func() {
		b.mu.Lock()
		if b.claimGen == gen {
			b.claimGen++
			b.console, b.claimed = nil, false
		}
		b.mu.Unlock()
	}
}

// Claimed reports whether something owns the port — so the UI can say why the
// console is quiet rather than appearing broken.
func (b *Bridge) Claimed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.claimed
}

// Type sends bytes to the node's serial input, as if someone had typed them.
//
// The caller supplies the line ending, because which one a command needs is the
// firmware's business and differs between applications.
func (b *Bridge) Type(input []byte) error {
	if err := b.send(kindConsoleIn, input); err != nil {
		return fmt.Errorf("firmware: console input: %w", err)
	}
	return nil
}

// Deliver hands a frame to the emulated firmware — a frame the channel decided
// this node received. Only CRC-passing frames should be delivered; everything
// else belongs in the ledger and is withheld, exactly as on real hardware.
func (b *Bridge) Deliver(frame []byte) error {
	if err := b.send(kindFrame, frame); err != nil {
		return fmt.Errorf("firmware: deliver: %w", err)
	}
	return nil
}

func (b *Bridge) send(kind byte, payload []byte) error {
	b.mu.Lock()
	c, closed := b.conn, b.closed
	b.mu.Unlock()
	if closed {
		return ErrClosed
	}
	if c == nil {
		return errors.New("no node attached")
	}
	if len(payload) > 0xFFFF {
		return fmt.Errorf("payload of %d bytes exceeds the wire format", len(payload))
	}
	hdr := []byte{kind, byte(len(payload) >> 8), byte(len(payload))}
	_, err := c.Write(append(hdr, payload...))
	return err
}

// Attached reports whether an emulator is connected.
func (b *Bridge) Attached() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.conn != nil
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	wasOpen := !b.closed
	b.closed = true
	c := b.conn
	b.mu.Unlock()
	if wasOpen {
		close(b.done)
	}
	if c != nil {
		_ = c.Close()
	}
	return b.ln.Close()
}

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
