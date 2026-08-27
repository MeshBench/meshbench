package firmware

import (
	"io"
	"sync"
)

// A ConsoleSink is where an emulated node's serial output goes.
//
// Two destinations, because they answer different questions and neither can
// stand in for the other. The file is the whole boot chain from the first ROM
// byte, kept whether or not anybody is looking, and is what the board probe
// reads afterwards to decide what a board did. The tee is whoever is looking
// now - the console pane, a companion client, meshcore-cli over TCP - attached
// long after the machine started and detached again while it runs.
//
// The tee is resolved per write rather than captured once. An emulated node's
// serial port changes hands while it runs: the workbench console holds it,
// a client claims it, the claim is released. A writer captured when the pump
// started would go on feeding whoever held the port at boot, which is nobody.
type ConsoleSink struct {
	file io.Writer

	mu  sync.Mutex
	tee io.Writer
}

// NewConsoleSink writes the whole boot chain to file, and a live copy to
// whoever is looking (see SetTee).
func NewConsoleSink(file io.Writer) *ConsoleSink { return &ConsoleSink{file: file} }

func (s *ConsoleSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	tee := s.tee
	s.mu.Unlock()
	if tee != nil {
		// Best effort, as the bridge's own console arm is: a reader that
		// cannot keep up must not stall the node it is reading.
		_, _ = tee.Write(p)
	}
	if s.file == nil {
		return len(p), nil
	}
	return s.file.Write(p)
}

// SetTee directs a copy of everything the node says at w. Nil stops the copy.
func (s *ConsoleSink) SetTee(w io.Writer) {
	s.mu.Lock()
	s.tee = w
	s.mu.Unlock()
}
