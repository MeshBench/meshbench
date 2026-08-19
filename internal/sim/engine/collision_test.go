package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// A receiver has one demodulator, and two packets arriving together cannot
// both come out of it.
//
// This is the defect that made a flood behave like nothing on the air: with
// the judgement run once per transmitter-receiver pair, two colliding relays
// that each cleared their own threshold were both accepted and both handed to
// the firmware. Every repeater then had every copy, so nothing thinned out and
// a single message produced hundreds of relays a second - against 0.76 a
// second measured across the whole of the real ScotMesh network.
func TestOneReceiverDecodesOnePacketAtATime(t *testing.T) {
	for _, mode := range []engine.RFMode{engine.RFCalculated, engine.RFWaveform} {
		t.Run(string(mode), func(t *testing.T) {
			e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: mode})
			e.Add(wfNode("listener", 0, 22), nil)
			e.Add(wfNode("east", 0.010, 22), nil)
			e.Add(wfNode("west", -0.010, 22), nil)

			frame := make([]byte, 40)
			for i := range frame {
				frame[i] = byte(51 + i*3)
			}
			if err := e.Run(context.Background(), 10); err != nil {
				t.Fatal(err)
			}
			// Both neighbours key up on the same tick, as two repeaters
			// relaying the same flood do.
			e.InjectFrame(1, frame)
			e.InjectFrame(2, frame)
			if err := e.Run(context.Background(), 5000); err != nil {
				t.Fatal(err)
			}

			decoded := 0
			for _, ev := range e.Events() {
				if ev.Kind == "rx" && ev.To == "listener" {
					decoded++
				}
			}
			if decoded > 1 {
				t.Fatalf("one receiver demodulated %d simultaneous packets; "+
					"a LoRa radio has one demodulator", decoded)
			}
		})
	}
}

// Losing the demodulator is a cause, and the ledger has to say so. A miss with
// no reason is the thing this project treats as a bug in itself.
func TestABusyDemodulatorSaysSo(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	e.Add(wfNode("listener", 0, 22), nil)
	e.Add(wfNode("east", 0.010, 22), nil)
	e.Add(wfNode("west", -0.010, 22), nil)

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(90 + i)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(1, frame)
	if err := e.Run(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(2, frame) // arrives while the first is still being received
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}

	for _, ev := range e.Events() {
		if ev.Kind == "miss" && ev.To == "listener" &&
			strings.Contains(ev.Detail, "demodulator was already locked") {
			return
		}
	}
	t.Fatal("nothing in the ledger explained the second packet's fate; " +
		"a receiver that was busy should say which packet had it")
}

// Collision corruption: a packet can win the demodulator, lead every
// interferer on average, and still lose because something landed on its
// symbols part-way through.
//
// The calculated path had no way to express that - an interferer either failed
// one ratio for the whole packet or may as well not have transmitted - so a
// mid-packet collision was waved through. Waveform mode has always got this
// right, because the interferer is summed into the window as IQ.
func TestAMidPacketCollisionCorrupts(t *testing.T) {
	outcome := func(interfererStartMs uint32, lonOffset float64) (string, string) {
		t.Helper()
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
		e.Add(wfNode("a", 0, 22), nil)
		e.Add(wfNode("b", 0.010, 22), nil)
		e.Add(wfNode("c", lonOffset, 22), nil)

		frame := make([]byte, 40)
		for i := range frame {
			frame[i] = byte(37 + i*11)
		}
		if err := e.Run(context.Background(), 10); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(0, frame)
		if err := e.Run(context.Background(), interfererStartMs); err != nil {
			t.Fatal(err)
		}
		e.InjectFrame(2, frame[:20])
		if err := e.Run(context.Background(), 8000); err != nil {
			t.Fatal(err)
		}
		for _, ev := range e.Events() {
			if ev.From == "a" && ev.To == "b" {
				return ev.Kind, ev.Detail
			}
		}
		return "nothing", ""
	}

	// An equal-power interferer landing in the middle of the packet: b cannot
	// capture over it, so the symbols underneath are gone.
	kind, detail := outcome(200, 0.020)
	if kind != "miss" {
		t.Fatalf("a mid-packet collision it could not capture over decoded anyway "+
			"(%s: %s)", kind, detail)
	}
	if !strings.Contains(detail, "destroyed by a collision") {
		t.Fatalf("the miss did not name collision damage as the cause: %q", detail)
	}

	// Clear air, same everything else.
	if kind, detail := outcome(6000, 0.020); kind != "rx" {
		t.Fatalf("with the air clear this should decode (%s: %s)", kind, detail)
	}
}

// Capture still works, which is the half that must not break. A close, strong
// transmitter is not stopped by a distant one keying up mid-packet - otherwise
// this is just "overlapping packets fail", which is the rule the channel is
// forbidden from having.
func TestCaptureSurvivesACollisionItLeads(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	e.Add(wfNode("near", 0.001, 22), nil) // ~70 m from the listener
	e.Add(wfNode("listener", 0, 22), nil) //
	e.Add(wfNode("far", 0.090, 22), nil)  // several km away: far weaker here

	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(200 - i)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, frame)
	if err := e.Run(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(2, frame[:20]) // lands mid-packet, but far below capture
	if err := e.Run(context.Background(), 8000); err != nil {
		t.Fatal(err)
	}

	for _, ev := range e.Events() {
		if ev.From == "near" && ev.To == "listener" {
			if ev.Kind != "rx" {
				t.Fatalf("a signal well clear of its interferer failed anyway "+
					"(%s: %s); capture effect has stopped working", ev.Kind, ev.Detail)
			}
			return
		}
	}
	t.Fatal("no event from near to listener at all")
}

// No reception may report an SNR the hardware could not have reported. The
// ceiling is measured: 1,992 receptions from the real ScotMesh network have a
// hard wall at +15 dB and not one above it, against +94.1 dB here.
func TestNoReceptionReportsAnImpossibleSNR(t *testing.T) {
	for _, mode := range []engine.RFMode{engine.RFCalculated, engine.RFWaveform} {
		t.Run(string(mode), func(t *testing.T) {
			e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: mode})
			// Two nodes almost on top of each other: the arithmetic here runs
			// far past anything a modem can express.
			e.Add(wfNode("hilltop", 0, 22), nil)
			e.Add(wfNode("next to it", 0.0002, 22), nil)

			frame := make([]byte, 32)
			for i := range frame {
				frame[i] = byte(i * 7)
			}
			if err := e.Run(context.Background(), 10); err != nil {
				t.Fatal(err)
			}
			e.InjectFrame(0, frame)
			if err := e.Run(context.Background(), 5000); err != nil {
				t.Fatal(err)
			}

			seen := false
			for _, ev := range e.Events() {
				if ev.Kind != "rx" && ev.Kind != "miss" {
					continue
				}
				seen = true
				if ev.SNRdB > dsp.ReportableSNRCeilingDB {
					t.Fatalf("%s -> %s reported %.1f dB, which no SX126x can say "+
						"(ceiling %.1f dB)", ev.From, ev.To, ev.SNRdB,
						dsp.ReportableSNRCeilingDB)
				}
			}
			if !seen {
				t.Fatal("no reception at all between two adjacent nodes")
			}
		})
	}
}
