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

// The locking contest: two preambles inside the detector's commitment window,
// and the dominant one wins - not the one that keyed up first.
//
// This is capture effect at the moment it actually happens. Measurement
// studies on real chips put the commit at about four preamble symbols; inside
// that window the receiver has not chosen yet, and first-come-keeps-it would
// systematically hand the packet to timing over power, which is the opposite
// of what the phenomenon means.
func TestTheStrongerPacketWinsTheLockingContest(t *testing.T) {
	for _, mode := range []engine.RFMode{engine.RFCalculated, engine.RFWaveform} {
		t.Run(string(mode), func(t *testing.T) {
			e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: mode})
			e.Add(wfNode("listener", 0, 22), nil)
			e.Add(wfNode("weak", 0.060, 22), nil)   // ~4 km out
			e.Add(wfNode("strong", 0.001, 22), nil) // ~70 m out

			frame := make([]byte, 40)
			for i := range frame {
				frame[i] = byte(140 + i)
			}
			if err := e.Run(context.Background(), 10); err != nil {
				t.Fatal(err)
			}
			e.InjectFrame(1, frame) // the weak one keys up first...
			if err := e.Run(context.Background(), 20); err != nil {
				t.Fatal(err)
			}
			e.InjectFrame(2, frame) // ...and the strong one lands 10 ms later,
			// inside the ~11 ms the detector spends deciding at SF8/125k.
			if err := e.Run(context.Background(), 8000); err != nil {
				t.Fatal(err)
			}

			for _, ev := range e.Events() {
				if ev.From == "strong" && ev.To == "listener" {
					if ev.Kind != "rx" {
						t.Fatalf("the dominant signal lost the locking contest to an "+
							"earlier weak one (%s: %s)", ev.Kind, ev.Detail)
					}
					return
				}
			}
			t.Fatal("no event from strong to listener at all")
		})
	}
}

// A lock ends when its packet does. MeshCore preambles are long - 32 symbols
// at SF8 - and a holder that falls silent early in one leaves plenty for the
// detector to acquire, exactly as our own Detect would on the samples. A rule
// that killed the packet on any overlap at all would quietly fail every
// staggered relay whose predecessor was short.
func TestAFreedDemodulatorCatchesWhatIsLeftOfThePreamble(t *testing.T) {
	for _, mode := range []engine.RFMode{engine.RFCalculated, engine.RFWaveform} {
		t.Run(string(mode), func(t *testing.T) {
			e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: mode})
			e.Add(wfNode("listener", 0, 22), nil)
			e.Add(wfNode("early", 0.060, 22), nil) // ~4 km: heard, but 16 dB down on late
			e.Add(wfNode("late", 0.010, 22), nil)  // ~600 m

			frame := make([]byte, 40)
			for i := range frame {
				frame[i] = byte(60 + i*3)
			}
			if err := e.Run(context.Background(), 10); err != nil {
				t.Fatal(err)
			}
			e.InjectFrame(1, frame[:8]) // a short packet: off the air quickly
			if err := e.Run(context.Background(), 130); err != nil {
				t.Fatal(err)
			}
			// The late packet starts well past the contest window, while the
			// early one is still up - but the early one ends soon enough that
			// most of the late preamble is clean air.
			e.InjectFrame(2, frame)
			if err := e.Run(context.Background(), 8000); err != nil {
				t.Fatal(err)
			}

			for _, ev := range e.Events() {
				if ev.From == "late" && ev.To == "listener" {
					if ev.Kind != "rx" {
						t.Fatalf("a demodulator freed early in the preamble did not "+
							"re-acquire (%s: %s)", ev.Kind, ev.Detail)
					}
					return
				}
			}
			t.Fatal("no event from late to listener at all")
		})
	}
}

// Reported receive power stays inside the register that reports it: the
// SX126x's RssiPkt spans 0 to -127.5 dBm, and the 2,000 real ScotMesh packets
// span exactly -127 to 0. Two co-located nodes must not publish +18 dBm.
func TestNoReceptionReportsAnImpossibleRSSI(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	e.Add(wfNode("hilltop", 0, 22), nil)
	e.Add(wfNode("next to it", 0.0002, 22), nil)

	frame := make([]byte, 32)
	for i := range frame {
		frame[i] = byte(i * 5)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, frame)
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}

	seen := false
	for _, r := range e.Ledger.Rows() {
		seen = true
		if r.RSSIdBm > dsp.ReportableRSSICeilingDBm || r.RSSIdBm < dsp.ReportableRSSIFloorDBm {
			t.Fatalf("%s -> %s reported %.1f dBm, outside the register's 0..-127.5",
				r.FromNode, r.ToNode, r.RSSIdBm)
		}
	}
	if !seen {
		t.Fatal("no reception at all between two adjacent nodes")
	}
}
