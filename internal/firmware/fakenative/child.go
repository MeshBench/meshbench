package fakenative

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// The bridge's wire format, from this side of the socket.
//
// A second copy of these numbers, deliberately: MeshCore's own shim carries a
// third in C++, and the format is what the two agree on rather than something
// either imports. A stand-in that shared the host's constants could not catch
// the host changing them.
const (
	kindFrame      = 0x01
	kindTick       = 0x02
	kindAck        = 0x03
	kindConsoleIn  = 0x06
	kindConsoleOut = 0x07
)

// stuckLimit is how long a ModeStuck child holds on before giving up of its
// own accord.
//
// It exists only so a test that is killed between starting a child and killing
// it cannot leave one behind for ever. On Linux the backend's PDEATHSIG has
// already covered that; this covers the rest.
const stuckLimit = 2 * time.Minute

// dialTimeout is how long the stand-in waits for the bridge it was pointed at.
// The listener is already open before the child is launched, so this only ever
// fires on an address that was never going to answer.
const dialTimeout = 30 * time.Second

// Serve runs this process as a stand-in for a native node and returns the
// status it should exit with. A test binary's TestMain calls it when Mode is
// not empty.
func Serve() int {
	// The words a published build prints, not words of our own.
	//
	// A stand-in that says something different is a trap for the next person:
	// they write a test against what the stand-in says, and it passes against
	// a node that never says it. Checked against repeater-v1.17.1 and main.
	fmt.Fprintln(os.Stderr, BootLine)
	switch Mode() {
	case ModeExit:
		return 0
	case ModeCrash:
		return CrashStatus
	case ModeStuck:
		signal.Ignore(syscall.SIGTERM, syscall.SIGINT)
		time.Sleep(stuckLimit)
		return 0
	}
	return attach()
}

// attach connects back to the bridge and runs the node's whole life on the
// clock the engine supplies.
func attach() int {
	addr := argValue("--bridge")
	if addr == "" {
		fmt.Fprintln(os.Stderr, "no --bridge address")
		return 2
	}
	// With a deadline, because a test that mistypes the address should see a
	// child that gave up and said so rather than one that sits there being
	// waited for by whatever budget the caller happened to set.
	dialCtx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bridge dial:", err)
		return 2
	}
	defer func() { _ = c.Close() }()

	advertAt, adverted := txAtMs(), Mode() != ModeAdvert
	var reached uint32
	var hdr [3]byte
	for {
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			// The socket going away is how a node is told to stop, so this is
			// the ordinary way out and not an error. The simulated time it got
			// to goes with it, as a real node's does: a node that processed
			// every tick and one that stopped at the first close identically,
			// and the number is what tells them apart.
			fmt.Fprintf(os.Stderr, ClosedLine+"\n", reached)
			return 0
		}
		payload := make([]byte, binary.BigEndian.Uint16(hdr[1:]))
		if len(payload) > 0 {
			if _, err := io.ReadFull(c, payload); err != nil {
				fmt.Fprintf(os.Stderr, ClosedLine+"\n", reached)
				return 0
			}
		}
		switch {
		case hdr[0] == kindConsoleIn:
			// Echoed with the firmware's own prompt, because a console reply is
			// the one thing a timer cannot produce and several phases judge a
			// board on exactly that.
			if err := send(c, kindConsoleOut, append([]byte("-> "), payload...)); err != nil {
				return 1
			}
		case hdr[0] != kindTick || len(payload) != 4:
			// Anything else is the host's business, not a node's.
		default:
			reached = binary.BigEndian.Uint32(payload)
			if !adverted && reached >= advertAt {
				adverted = true
				// Sent before the acknowledgement, so the engine collects it on
				// the tick it belongs to rather than the one after.
				if err := send(c, kindFrame, advertFrame()); err != nil {
					return 1
				}
			}
			if err := send(c, kindAck, payload); err != nil {
				return 1
			}
		}
	}
}

// advertFrame is something for the channel to carry. Not a MeshCore packet:
// nothing here parses one, and a stand-in that pretended to speak the mesh
// would be a second, wrong implementation of it.
func advertFrame() []byte {
	return []byte{0x12, 0x00, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7}
}

func send(c net.Conn, kind byte, payload []byte) error {
	hdr := []byte{kind, byte(len(payload) >> 8), byte(len(payload))}
	_, err := c.Write(append(hdr, payload...))
	return err
}

// argValue reads a `--name value` pair out of this process's own arguments.
//
// By hand rather than through flag, because the arguments were written for
// MeshCore and a test binary's own flag set would reject the ones it has never
// heard of before any test could run.
func argValue(name string) string {
	for i, a := range os.Args {
		if a == name && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}
