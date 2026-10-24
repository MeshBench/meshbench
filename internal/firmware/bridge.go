// Package firmware connects emulated nodes to the RF engine.
//
// An emulated node runs inside Renode or QEMU, and the chip is inside the
// emulator with it: Renode's tools/renode/peripherals/VirtualSX1262.cs and
// QEMU's hw/ssi/sx1262.c both load the same virtual-sx1262 a native node links.
// This socket carries what is not in there - the air - so the emulator hands
// transmitted frames out over it and takes delivered frames back.
//
// A socket rather than an emulator-native wireless medium is deliberate. The
// physics lives in internal/rf, and an emulated node must share exactly the
// same channel as a native one: otherwise the two backends are not comparable,
// which is the entire reason ADR-0010 has both.
package firmware

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
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

	// kindFrameFailed is host → node: a frame this receiver locked onto and
	// could not trust. The first payload byte says where it failed, the rest is
	// the frame as it arrived. A real SX1262 says so - HeaderErr for a header
	// it could not read, RxDone beside CrcErr for a payload it could not - and
	// a firmware that never hears either has error paths nothing exercises.
	// Sent only to a node whose radio stats record carries capFrameFailed: a
	// native bridge built before this kind ends its connection on a kind it
	// does not know, so a node that never says it understands is never sent it
	// and sees what it always saw - nothing.
	kindFrameFailed = 0x0A

	// capFrameFailed is the bit in byte 47 of the radio stats record by which a
	// node says it takes kindFrameFailed.
	capFrameFailed = 0x01
	// capNoLock is the bit by which it says its chip reads FailedNoLock. A chip
	// older than that reads the code as a CRC error and would raise RxDone for
	// a frame it never locked onto, so the code goes only where this is set.
	capNoLock = 0x02
)

// FrameFailure is where a frame the receiver locked onto went wrong, numbered
// as virtual-sx1262's vsx_deliver_frame_failed numbers it.
type FrameFailure byte

const (
	// FailedCRC is a header that decoded and a payload that did not: the chip
	// raises RxDone beside CrcErr and the corrupt bytes are in its buffer.
	FailedCRC FrameFailure = 1
	// FailedHeader is a header the receiver could not read: the chip raises
	// HeaderErr, and no RxDone follows.
	FailedHeader FrameFailure = 2
	// FailedNoLock is a frame the receiver could detect and never locked onto.
	// The part raises nothing for it and leaves PreambleDetected latched; the
	// chip model withdraws the HeaderValid it raised from the carrier's length,
	// because that was its stand-in for a header it cannot decode.
	FailedNoLock FrameFailure = 3
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
	// backlog is what the node said while nobody held the port, kept for
	// whoever opens it next.
	backlog []byte
	// claimed marks the port as owned by a companion link, and claimGen
	// counts claims so a release can tell whether its own is still current.
	claimed  bool
	claimGen uint64

	ln   net.Listener
	node string

	// hasConsole is what the backend answered when the node was started, and
	// consoleIn is the backend's own serial port when it has one.
	hasConsole bool
	consoleIn  io.Writer

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

	// takesFailed is whether the node has said it understands kindFrameFailed.
	// A native bridge built before that kind closes its connection on any kind
	// it does not know, so it is sent only to a node that has asked for it.
	takesFailed bool
	takesNoLock bool

	// done is closed when the bridge is. Anything waiting on the node must
	// select on it: a node whose process has gone will never ack, and a wait
	// with no way to hear about that hangs for as long as its deadline allows -
	// or for ever, if it was given none.
	done chan struct{}

	// tickAt is the simulated time of the last tick sent. A frame the node
	// writes while running that tick arrives before its ack, so this is the
	// moment anything the node says was said - read from whichever goroutine
	// is handling the output, without the engine's lock.
	tickAt atomic.Uint32
}

// Now is the simulated time of the tick the node is running, or last ran.
func (b *Bridge) Now() uint32 { return b.tickAt.Load() }

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
			b.writeConsole(buf)

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
				if n >= 39 {
					st.DIO1Mask = binary.BigEndian.Uint16(buf[37:])
					st.DIO1Reported = true
				}
				// The console's line rate, which the firmware sets by writing a
				// divider the machine turns back into a number. Zero where the
				// board's console is not on a UART at all - a USB peripheral has
				// no line and no rate - and zero, too, from an emulator older
				// than the field, which is why it is read on length like the
				// rest of this tail.
				if n >= 43 {
					st.ConsoleBaud = binary.BigEndian.Uint32(buf[39:])
				}
				// What the firmware's IrqMask kept out, from a chip new enough
				// to apply the mask at all.
				if n >= 47 {
					st.IRQSuppressed = binary.BigEndian.Uint32(buf[43:])
					st.SuppressedReported = true
				}
				b.mu.Lock()
				b.stats = st
				b.takesFailed = n >= 48 && buf[47]&capFrameFailed != 0
				b.takesNoLock = n >= 48 && buf[47]&capNoLock != 0
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

// Deliver hands a frame to the emulated firmware — a frame the channel decided
// this node received. Only CRC-passing frames go this way; a frame the receiver
// locked onto and could not trust goes through DeliverFailed, and one it never
// locked onto reaches the firmware not at all, exactly as on real hardware.
func (b *Bridge) Deliver(frame []byte) error {
	if err := b.send(kindFrame, frame); err != nil {
		return fmt.Errorf("firmware: deliver: %w", err)
	}
	return nil
}

// DeliverFailed tells the firmware's chip that a frame it locked onto failed,
// and where. Nothing is decoded from it; the frame rides along because a CRC
// failure leaves the corrupt bytes in the chip's buffer, as the part does.
//
// A node that has not said it takes the kind is told nothing, which is what it
// was always told.
func (b *Bridge) DeliverFailed(frame []byte, why FrameFailure) error {
	b.mu.Lock()
	takes := b.takesFailed && (why != FailedNoLock || b.takesNoLock)
	b.mu.Unlock()
	if !takes {
		return nil
	}
	if err := b.send(kindFrameFailed, append([]byte{byte(why)}, frame...)); err != nil {
		return fmt.Errorf("firmware: deliver failed frame: %w", err)
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
