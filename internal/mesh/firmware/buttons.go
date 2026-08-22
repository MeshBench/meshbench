package firmware

import (
	"context"
	"net"
	"os"
	"sync"
)

// ButtonSender works a board's inputs: its buttons, its keyboard, its touch
// panel.
//
// The other half of the panel. A board's lamps are outputs to watch and these
// are inputs to drive, and the interface that draws one should be able to work
// the other.
//
// It listens and the emulator connects, as the display does, because the
// emulator is started by us and dies with us - the side that outlives the
// other should own the address. More than one device connects: the buttons,
// the keyboard and the touch panel are separate peripherals inside the
// machine, so every message goes to all of them and each keeps the ones
// tagged for it.
type ButtonSender struct {
	mu    sync.Mutex
	conns []net.Conn
	// held is what each pin was last driven to, so a caller can ask what is
	// pressed without the guest being the only record of it.
	held map[int]bool

	ln   net.Listener
	path string
	done chan struct{}
}

// Every message is eight bytes: a tag and seven of payload. Fixed width
// because several devices read this socket and each has to skip what is not
// its own without knowing how long it was.
const (
	msgLen = 8
	// buttonTag carries a pin and a level. The pin rather than an index into
	// anything, so neither end has to keep a list in step with the other.
	buttonTag = 'B'
	keyTag    = 'K'
	touchTag  = 'T'
	// analogTag carries a converter channel and what it reads, for the parts
	// of a board the simulation drives rather than a person: on these boards,
	// the divider its cell is measured through.
	analogTag = 'A'
)

// ListenButtons starts accepting the emulator's connection at path.
func ListenButtons(path string) (*ButtonSender, error) {
	_ = os.Remove(path)
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}
	b := &ButtonSender{ln: ln, path: path, done: make(chan struct{}),
		held: map[int]bool{}}
	go b.accept()
	return b, nil
}

// Path is where the emulator should connect.
func (b *ButtonSender) Path() string { return b.path }

// Press holds a button down or lets it go.
//
// down is what a finger does, not what the pin reads: a button with a pull-up
// reads low while it is held, and every board here is wired that way. Keeping
// the translation in one place is what stops a board being drawn permanently
// pressed because somebody inverted it twice.
func (b *ButtonSender) Press(pin int, down bool) error {
	level := byte(1)
	if down {
		level = 0
	}
	if err := b.send([msgLen]byte{buttonTag, byte(pin), level}); err != nil {
		return err
	}
	b.mu.Lock()
	b.held[pin] = down
	b.mu.Unlock()
	return nil
}

// Key types one character at the board's keyboard.
//
// A character rather than a scan code, because that is what this keyboard
// sends: on these boards it is a second microcontroller that has already
// decided what was pressed.
func (b *ButtonSender) Key(ch byte) error {
	if ch == 0 {
		return nil
	}
	return b.send([msgLen]byte{keyTag, ch})
}

// Touch reports where the panel is being touched, or that it is not.
func (b *ButtonSender) Touch(x, y int, down bool) error {
	var d byte
	if down {
		d = 1
	}
	return b.send([msgLen]byte{touchTag,
		byte(x), byte(x >> 8), byte(y), byte(y >> 8), d})
}

// Analog is what one of the board's converter channels reads from now on.
func (b *ButtonSender) Analog(channel int, raw uint16) error {
	return b.send([msgLen]byte{analogTag, byte(channel), byte(raw), byte(raw >> 8)})
}

// send puts one message to every device listening, and reports whether any
// heard it.
func (b *ButtonSender) send(msg [msgLen]byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.conns) == 0 {
		return errNoButtons
	}
	live := b.conns[:0]
	sent := false
	for _, c := range b.conns {
		if _, err := c.Write(msg[:]); err != nil {
			_ = c.Close()
			continue
		}
		live = append(live, c)
		sent = true
	}
	b.conns = live
	if !sent {
		return errNoButtons
	}
	return nil
}

// Held reports whether a pin is being held down.
func (b *ButtonSender) Held(pin int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.held[pin]
}

// Ready reports whether anything is listening on the other end. A press with
// nothing attached is worth saying out loud rather than swallowing.
func (b *ButtonSender) Ready() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.conns) > 0
}

// Close stops listening and removes the socket.
func (b *ButtonSender) Close() error {
	select {
	case <-b.done:
	default:
		close(b.done)
	}
	err := b.ln.Close()
	_ = os.Remove(b.path)
	return err
}

func (b *ButtonSender) accept() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.conns = append(b.conns, conn)
		// A board that restarts comes up with its buttons released, whatever
		// they were before, so the record starts again with it.
		b.held = map[int]bool{}
		b.mu.Unlock()
	}
}

var errNoButtons = errNoButtonsError{}

type errNoButtonsError struct{}

func (errNoButtonsError) Error() string {
	return "firmware: this board is not running, so nothing on it can be worked"
}

// ErrNoButtons is that error, for callers that want to tell it apart from a
// board that has no buttons at all.
func ErrNoButtons() error { return errNoButtons }
