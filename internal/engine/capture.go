package engine

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/A13xB0/meshcoresim/internal/capture"
)

// Capture writes every reception to a pcapng file as the run happens.
//
// One file, every receiver's view of every frame, distinguished by the
// pseudo-header's ToNode — which is the whole reason to capture from a
// simulator rather than from a radio. A real capture has one vantage point; a
// packet node A heard while node B did not is the most informative event in a
// mesh, and no single receiver can record it.
type Capture struct {
	mu sync.Mutex
	// sink is whatever the frames go to: a file, or a pipe writer that drops
	// rather than blocking when nothing is reading it.
	sink io.WriteCloser
	// name is what to call this capture in the UI; a pipe has no Name().
	name   string
	w      *capture.PcapngWriter
	ids    map[string]uint16
	frames int
	// udp sends each view as its own datagram instead of into a pcapng stream.
	udp bool
}

// newCaptureOn wraps an already-open file in the pcapng writer — shared by
// the file capture and the FIFO capture, which differ only in how the file
// came to exist.
func newCaptureOn(sink io.WriteCloser) (*Capture, error) {
	w, err := capture.NewPcapngWriter(sink)
	if err != nil {
		return nil, fmt.Errorf("engine: capture: %w", err)
	}
	return &Capture{sink: sink, w: w, ids: map[string]uint16{}}, nil
}

// StartCaptureUDP sends every receiver's view to a UDP port on loopback.
//
// Simpler than the named pipe in every way that was causing trouble. A pcapng
// stream carries its section header once, at the very beginning, so a reader
// that attaches later - or a second one - sees a stream it cannot parse and
// displays nothing at all, however much traffic is flowing. Datagrams have no
// such history: Wireshark can be started, stopped and restarted mid-run and
// simply picks up from the next packet.
//
// The payload is the same pseudo-header and frame the pcapng link layer
// carries, so one dissector reads both.
func (e *Engine) StartCaptureUDP(addr string) error {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return fmt.Errorf("engine: udp capture: %w", err)
	}
	c := &Capture{sink: conn, name: addr, ids: map[string]uint16{}, udp: true}
	e.mu.Lock()
	old := e.capture
	e.capture = c
	e.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
	return nil
}

// StartCapture begins writing to path, replacing anything already there.
func (e *Engine) StartCapture(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("engine: capture: %w", err)
	}
	c, err := newCaptureOn(f)
	if c != nil {
		c.name = path
	}
	if err != nil {
		_ = f.Close()
		return err
	}
	e.mu.Lock()
	old := e.capture
	e.capture = c
	e.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
	return nil
}

// StopCapture closes the file and reports what was written.
func (e *Engine) StopCapture() (path string, frames int, err error) {
	e.mu.Lock()
	c := e.capture
	e.capture = nil
	e.mu.Unlock()
	if c == nil {
		return "", 0, nil
	}
	c.mu.Lock()
	path, frames = c.name, c.frames
	c.mu.Unlock()
	return path, frames, c.close()
}

// CapturePath is the file being written, or empty.
func (e *Engine) CapturePath() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.capture == nil {
		return ""
	}
	return e.capture.name
}

func (c *Capture) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sink == nil {
		return nil
	}
	f := c.sink
	c.sink = nil
	return f.Close()
}

// nodeID assigns each node a stable small integer for the pseudo-header.
func (c *Capture) nodeID(name string) uint16 {
	id, ok := c.ids[name]
	if !ok {
		id = uint16(len(c.ids) + 1)
		c.ids[name] = id
	}
	return id
}

// write records one node's view of one frame.
func (c *Capture) write(atMs uint32, from, to string, p phy, rssi, snr float64,
	outcome capture.Outcome, crcOK bool, frame []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sink == nil {
		return
	}
	crc := uint8(0)
	if crcOK {
		crc = 1
	}
	h := capture.PseudoHeader{
		Version:  capture.PseudoHeaderVersion,
		Outcome:  capture.OutcomeCode(outcome),
		FromNode: c.nodeID(from), ToNode: c.nodeID(to),
		RSSIdBm: int16(rssi * 10), SNRdB: int16(snr * 10),
		FreqHz: uint32(p.freqMHz * 1e6), SF: uint8(p.sf),
		BWkHz: uint16(p.bandwidthHz / 1000), CR: uint8(p.codingRate), CRCOK: crc,
	}
	if c.udp {
		// One datagram per view. Dropped silently if nothing is listening,
		// which is what a diagnostic should do.
		buf := append(h.Encode(), frame...)
		if _, err := c.sink.Write(buf); err != nil {
			_ = c.sink.Close()
			c.sink = nil
		}
		c.frames++
		return
	}
	// Microseconds since the run began, not wall time: a capture of a
	// simulation is about simulated time, and stamping it with the clock on the
	// wall makes two runs of the same scenario incomparable.
	if err := c.w.WritePacket(uint64(atMs)*1000, h, frame); err != nil {
		// A capture that cannot be written is closed rather than retried every
		// packet: a full disk should cost one message, not a million.
		_ = c.sink.Close()
		c.sink = nil
		return
	}
	c.frames++
}
