package firmware

import (
	"context"
	"net"
	"os"
	"sync"
)

// ButtonSender presses a board's buttons.
//
// The other half of the panel: a board's lamps are outputs to watch and its
// buttons are inputs to drive, and the interface that draws one should be able
// to work the other.
//
// It listens and the emulator connects, as the display does, because the
// emulator is started by us and dies with us - the side that outlives the
// other should own the address.
type ButtonSender struct {
	mu   sync.Mutex
	conn net.Conn
	// held is what each pin was last driven to, so a caller can ask what is
	// pressed without the guest being the only record of it.
	held map[int]bool

	ln   net.Listener
	path string
	done chan struct{}
}

// buttonMsg is a tag, the pin, the level, and a byte of padding. The pin
// rather than an index into anything, so neither end has to keep a list in
// step with the other.
const buttonTag = 'B'

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
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return errNoButtons
	}
	level := byte(1)
	if down {
		level = 0
	}
	if _, err := b.conn.Write([]byte{buttonTag, byte(pin), level, 0}); err != nil {
		b.conn = nil
		return err
	}
	b.held[pin] = down
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
	return b.conn != nil
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
		if b.conn != nil {
			_ = b.conn.Close()
		}
		b.conn = conn
		// A board that restarts comes up with its buttons released, whatever
		// they were before, so the record starts again with it.
		b.held = map[int]bool{}
		b.mu.Unlock()
	}
}

var errNoButtons = errNoButtonsError{}

type errNoButtonsError struct{}

func (errNoButtonsError) Error() string {
	return "firmware: this board is not running, so its buttons cannot be pressed"
}

// ErrNoButtons is that error, for callers that want to tell it apart from a
// board that has no buttons at all.
func ErrNoButtons() error { return errNoButtons }
