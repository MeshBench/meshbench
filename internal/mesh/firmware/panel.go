package firmware

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
)

// PanelFrame is one picture from a board's display, as the panel stores it.
//
// Bits rather than pixels, because that is what a monochrome controller holds
// and converting here would throw away the only thing that makes the picture
// honest: it is exactly what the firmware drew, at the size it drew it.
type PanelFrame struct {
	Width, Height int
	// On is the display's own power state. A blank frame with On true is a
	// screen the firmware cleared; a blank frame with On false is a screen it
	// switched off, and MeshCore switches it off after an idle. Drawing those
	// the same way would report a sleeping board as a broken one.
	On bool
	// Bits is one bit a pixel, in the controller's page order: byte n holds
	// eight vertical pixels of column n%Width in page n/Width.
	Bits []byte
}

// Lit reports whether a pixel is on.
func (f *PanelFrame) Lit(x, y int) bool {
	if f == nil || x < 0 || y < 0 || x >= f.Width || y >= f.Height {
		return false
	}
	i := (y/8)*f.Width + x
	if i >= len(f.Bits) {
		return false
	}
	return f.Bits[i]&(1<<(y%8)) != 0
}

// panelMagic is the four bytes every frame starts with. Checked rather than
// assumed: this socket is a private contract between two of our own processes,
// and the failure it protects against is not an attacker but a version skew
// that would otherwise draw garbage and look like a firmware bug.
var panelMagic = [4]byte{'M', 'B', 'F', '1'}

// PanelListener accepts frames from one node's display.
//
// It listens rather than connects, and the emulator's device connects to it,
// for the same reason the radio model works that way: the emulator is started
// by us and dies with us, so the side that outlives the other should own the
// address.
type PanelListener struct {
	mu    sync.Mutex
	frame *PanelFrame
	seq   uint64

	ln   net.Listener
	path string
	done chan struct{}
}

// ListenPanel starts accepting frames at path.
func ListenPanel(path string) (*PanelListener, error) {
	// A stale socket from a run that did not clean up would refuse the bind
	// and leave the board with no display for no reason anybody could see.
	_ = os.Remove(path)
	// ListenConfig rather than net.Listen, which the linter insists on and is
	// right to: a listener with no context is one nothing can cancel.
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}
	p := &PanelListener{ln: ln, path: path, done: make(chan struct{})}
	go p.accept()
	return p, nil
}

// Path is where the emulator should send its frames.
func (p *PanelListener) Path() string { return p.path }

// Frame is the last picture received, and a sequence number that changes when
// it does. Callers redraw on the sequence rather than comparing pictures.
func (p *PanelListener) Frame() (*PanelFrame, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frame, p.seq
}

// Close stops listening and removes the socket.
func (p *PanelListener) Close() error {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	err := p.ln.Close()
	_ = os.Remove(p.path)
	return err
}

func (p *PanelListener) accept() {
	for {
		conn, err := p.ln.Accept()
		if err != nil {
			select {
			case <-p.done:
				return
			default:
			}
			return
		}
		go p.read(conn)
	}
}

func (p *PanelListener) read(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	var hdr [8]byte
	for {
		if _, err := io.ReadFull(conn, hdr[:]); err != nil {
			return
		}
		if hdr[0] != panelMagic[0] || hdr[1] != panelMagic[1] ||
			hdr[2] != panelMagic[2] || hdr[3] != panelMagic[3] {
			return
		}
		w, h := int(hdr[4]), int(hdr[5])
		if w <= 0 || h <= 0 || h%8 != 0 {
			return
		}
		bits := make([]byte, w*h/8)
		if _, err := io.ReadFull(conn, bits); err != nil {
			return
		}
		p.mu.Lock()
		p.frame = &PanelFrame{Width: w, Height: h, On: hdr[7] != 0, Bits: bits}
		p.seq++
		p.mu.Unlock()
	}
}

// errNoPanel is returned where a caller asks for a display a board has not
// declared. Said rather than returning an empty picture, because an empty
// picture is what a working screen showing nothing looks like.
var errNoPanel = errors.New("firmware: this board declares no display")

// ErrNoPanel is that error, for callers that want to tell it apart.
func ErrNoPanel() error { return errNoPanel }
