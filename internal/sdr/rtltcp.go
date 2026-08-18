// An rtl_tcp server, so SDR++ can point at a simulated antenna.
//
// The protocol is rtl-sdr's own network format, chosen because SDR++ (and
// most SDR software) carries a native client for it and inventing a
// MeshBench protocol would help nobody (plan decision D4). Twelve bytes of
// header - "RTL0", a tuner id, a gain count - then a one-way stream of
// unsigned 8-bit IQ pairs, with five-byte commands coming back the other
// way. Eight bits is the format's ceiling: about 48 dB of dynamic range,
// which docs/shortcomings.md owns up to.
package sdr

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"time"
)

// SampleSource produces the stream: n complex baseband samples for the next
// span, at the source's own rate. Implementations decide what time means -
// the engine serves simulated time as it plays.
type SampleSource interface {
	NextSamples(n int) []complex128
	SampleRateHz() float64
}

// RTLTCP is one serving observer: a listener, one client at a time, exactly
// as rtl_tcp itself behaves.
type RTLTCP struct {
	ln net.Listener

	mu      sync.Mutex
	freqHz  uint32
	rateHz  uint32
	gainDB  uint32
	stopped bool
}

// ServeRTLTCP starts serving source at addr ("127.0.0.1:0" for an OS-picked
// port). It returns immediately; the listener runs until Close.
func ServeRTLTCP(addr string, source SampleSource) (*RTLTCP, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &RTLTCP{ln: ln}
	go s.acceptLoop(source)
	return s, nil
}

// Addr is where a client should point.
func (s *RTLTCP) Addr() string { return s.ln.Addr().String() }

// Tuned reports the client's last frequency and sample-rate commands - what
// SDR++ asked for, for the UI to show beside what the observer provides.
func (s *RTLTCP) Tuned() (freqHz, rateHz uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.freqHz, s.rateHz
}

func (s *RTLTCP) Close() error {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	return s.ln.Close()
}

func (s *RTLTCP) acceptLoop(source SampleSource) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		// One client at a time, like the real server: a second dongle user
		// gets the port refused rather than interleaved samples.
		s.serveClient(conn, source)
	}
}

func (s *RTLTCP) serveClient(conn net.Conn, source SampleSource) {
	defer func() { _ = conn.Close() }()

	// The dongle header: magic, tuner type (R820T, the common answer), and
	// a gain count the client uses to build its gain menu.
	hdr := make([]byte, 12)
	copy(hdr, "RTL0")
	binary.BigEndian.PutUint32(hdr[4:], 5)  // RTLSDR_TUNER_R820T
	binary.BigEndian.PutUint32(hdr[8:], 29) // gain steps
	if _, err := conn.Write(hdr); err != nil {
		return
	}

	// Commands arrive on their own goroutine; the sample stream must not
	// stall waiting on a read.
	go s.readCommands(conn)

	rate := source.SampleRateHz()
	chunk := int(rate / 20) // 50 ms of samples per write
	if chunk < 256 {
		chunk = 256
	}
	buf := make([]byte, chunk*2)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		stopped := s.stopped
		s.mu.Unlock()
		if stopped {
			return
		}
		iq := source.NextSamples(chunk)
		toU8(iq, buf)
		if _, err := conn.Write(buf); err != nil {
			return
		}
	}
}

// readCommands consumes the client's five-byte commands. Frequency and rate
// are recorded for the UI; gain is accepted and ignored, because a simulated
// front end has nothing to saturate yet.
func (s *RTLTCP) readCommands(conn net.Conn) {
	cmd := make([]byte, 5)
	for {
		if _, err := io.ReadFull(conn, cmd); err != nil {
			return
		}
		val := binary.BigEndian.Uint32(cmd[1:])
		s.mu.Lock()
		switch cmd[0] {
		case 0x01:
			s.freqHz = val
		case 0x02:
			s.rateHz = val
		case 0x04:
			s.gainDB = val
		}
		s.mu.Unlock()
	}
}

// toU8 converts complex baseband to rtl_tcp's unsigned 8-bit interleaved
// IQ. The scale is fixed so the noise floor sits a few counts above zero and
// a strong nearby transmitter approaches full scale - the same dynamic-range
// compromise a real dongle's ADC makes.
func toU8(iq []complex128, out []byte) {
	const scale = 2.0e6 // amplitude counts per unit; signals arrive ~1e-5..1e-7
	for i, v := range iq {
		out[i*2] = clampU8(real(v)*scale + 127.5)
		out[i*2+1] = clampU8(imag(v)*scale + 127.5)
	}
	for i := len(iq) * 2; i < len(out); i += 2 {
		out[i], out[i+1] = 127, 127
	}
}

func clampU8(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}
