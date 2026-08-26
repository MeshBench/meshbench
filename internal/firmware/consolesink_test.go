package firmware

import (
	"bytes"
	"sync"
	"testing"
)

// The point of the sink is that both destinations get everything, and that
// attaching a reader long after the node booted still works.
//
// Both halves were real faults. An emulated node's serial output went only to
// its log file, so the console pane, the companion client and meshcore-cli all
// read a port nothing had ever written to - a board that answered everything
// typed at it and appeared to say nothing back. And the listener changes while
// a node runs, so a writer captured when the pump started would feed whoever
// held the port at boot, which is nobody.
func TestTheSinkFeedsTheFileAndWhoeverIsListening(t *testing.T) {
	var file, first, second bytes.Buffer
	s := &consoleSink{file: &file}

	if _, err := s.Write([]byte("ets Jul 29 2019\n")); err != nil {
		t.Fatalf("write with nobody listening: %v", err)
	}
	s.setTee(&first)
	if _, err := s.Write([]byte("boot ok\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	s.setTee(&second)
	if _, err := s.Write([]byte("radio ok\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	s.setTee(nil)
	if _, err := s.Write([]byte("after\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The file has the whole boot chain whether or not anybody was there.
	want := "ets Jul 29 2019\nboot ok\nradio ok\nafter\n"
	if file.String() != want {
		t.Errorf("the log holds %q, want %q", file.String(), want)
	}
	if first.String() != "boot ok\n" {
		t.Errorf("the first listener saw %q, want just what arrived while it held the port",
			first.String())
	}
	if second.String() != "radio ok\n" {
		t.Errorf("the second listener saw %q; the port did not change hands", second.String())
	}
}

// A listener that fails must not stop the node. The bridge's own console arm
// is best effort for the same reason: whether the simulation is right cannot
// depend on whether somebody is reading the output.
func TestAFailingListenerDoesNotStopTheNode(t *testing.T) {
	var file bytes.Buffer
	s := &consoleSink{file: &file}
	s.setTee(errWriter{})
	if _, err := s.Write([]byte("still printing\n")); err != nil {
		t.Fatalf("a broken listener broke the node: %v", err)
	}
	if file.String() != "still printing\n" {
		t.Errorf("the log holds %q", file.String())
	}
}

// Output arrives on the pump's goroutine while the port changes hands on the
// caller's. Run under -race, this is the whole assertion.
func TestTheSinkSurvivesAttachingWhileItIsWriting(t *testing.T) {
	s := &consoleSink{file: &bytes.Buffer{}}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = s.Write([]byte("line\n"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			s.setTee(&bytes.Buffer{})
			s.setTee(nil)
		}
	}()
	wg.Wait()
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errClosed }

var errClosed = &closedErr{}

type closedErr struct{}

func (*closedErr) Error() string { return "closed" }
