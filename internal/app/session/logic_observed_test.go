package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The matcher's inputs: three nodes, two measured links, one spreading factor.
func residualsFixture() (*Sim, []state.Node, []state.Link) {
	s := &Sim{}
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		n := scenario.Node{Name: name}
		n.Radio.SpreadFactor = 8
		s.nodes = append(s.nodes, n)
	}
	nodes := []state.Node{{Name: "alpha"}, {Name: "bravo"}, {Name: "charlie"}}
	links := []state.Link{
		{A: 0, B: 1, MarginDB: 18, Known: true},
		{A: 1, B: 2, MarginDB: 6, Known: true},
	}
	return s, nodes, links
}

// A deep-path packet is direct evidence about its final relay's link.
//
// The old matcher filtered to hop count zero or one and paired origin with
// observer - so a packet three relays deep, whose SNR belongs entirely to the
// relay-observer link it was measured on, was thrown away. On a live ScotMesh
// page that filter plus the pairing left 0 matched of 182 attempted, out of
// 1,992 observations carrying SNR.
func TestADeepPathObservationMatchesItsTransmitter(t *testing.T) {
	s, nodes, links := residualsFixture()
	res := s.residualsOf([]state.Observed{
		{Receiver: "bravo", Transmitter: "alpha", Origin: "somewhere-far",
			HopCount: 3, HasSNR: true, SNRdB: 4},
	}, links, nodes)
	if res.Matched != 1 {
		t.Fatalf("matched = %d; a hop-3 packet is evidence about the link it "+
			"was heard on (%+v)", res.Matched, res)
	}
}

// An observation with no transmitter is a direct link only when nothing
// relayed it: at hop zero the origin is the transmitter, at any other depth
// the pair is unknowable and the sample must not be guessed at.
func TestAnOriginOnlyObservationNeedsHopZero(t *testing.T) {
	s, nodes, links := residualsFixture()
	res := s.residualsOf([]state.Observed{
		{Receiver: "bravo", Origin: "alpha", HopCount: 0, HasSNR: true, SNRdB: 4},
		{Receiver: "bravo", Origin: "alpha", HopCount: 2, HasSNR: true, SNRdB: 4},
	}, links, nodes)
	if res.Matched != 1 {
		t.Fatalf("matched = %d, want exactly the hop-zero sample", res.Matched)
	}
}

// Unmatched says why: a name outside the scenario and a pair with no measured
// link are different failures with different fixes, and their sum looking
// like one number is how a total matching failure went undiagnosed.
func TestUnmatchedSaysWhichFailureItWas(t *testing.T) {
	s, nodes, links := residualsFixture()
	res := s.residualsOf([]state.Observed{
		{Receiver: "bravo", Transmitter: "nobody-here", HasSNR: true, SNRdB: 4},
		{Receiver: "charlie", Transmitter: "alpha", HasSNR: true, SNRdB: 4},
	}, links, nodes)
	if res.OffScenario != 1 || res.NoLink != 1 {
		t.Fatalf("off-scenario %d, no-link %d; want 1 and 1 (%+v)",
			res.OffScenario, res.NoLink, res)
	}
	if res.Matched != 0 || res.Unmatched != 2 {
		t.Fatalf("matched %d unmatched %d", res.Matched, res.Unmatched)
	}
}

// A prediction past the modem's reporting ceiling is censored, not compared.
//
// Clamping it and letting it vote looked right and ran away: the clamped
// sample reports the same fixed residual whatever the excess term is, so the
// fitted median never moves and every calibration round adds the same few
// decibels for ever - measured live, +3.0 dB per round with no convergence.
// A censored sample says "at least this optimistic", and a bound does not
// get to vote a number.
func TestASaturatedPredictionIsCensoredNotVoting(t *testing.T) {
	s, nodes, _ := residualsFixture()
	// Margin +50 at SF8 (floor -10) is an unclamped prediction of +40 dB -
	// far past the +15 the modem can report. The +6 margin pair predicts
	// -4 dB, well inside the register, and is the only legitimate voter.
	links := []state.Link{
		{A: 0, B: 1, MarginDB: 50, Known: true},
		{A: 1, B: 2, MarginDB: 6, Known: true},
	}
	res := s.residualsOf([]state.Observed{
		{Receiver: "bravo", Transmitter: "alpha", HasSNR: true, SNRdB: 15},
		{Receiver: "charlie", Transmitter: "bravo", HasSNR: true, SNRdB: -6},
	}, links, nodes)
	if res.Matched != 2 || res.Censored != 1 {
		t.Fatalf("matched %d censored %d, want 2 and 1 (%+v)",
			res.Matched, res.Censored, res)
	}
	// The voter: predicted 6-10 = -4, observed -6, residual +2. If the
	// censored pair had voted its clamped zero, the median would sit between.
	if res.MedianDB != 2 {
		t.Fatalf("median = %+.1f dB, want the voter's own +2", res.MedianDB)
	}
}

// Calibration accumulates, because the residuals were measured on links that
// already carried the current term.
//
// The median is what the existing excess loss did NOT cover. Setting the
// total *to* the median - which this once did - silently discarded 20 dB of
// measured clutter in favour of 6 dB of leftover bias, and a calibrated model
// came out 14 dB more optimistic than an uncalibrated one.
func TestCalibrateAddsToWhatTheResidualsWereMeasuredOn(t *testing.T) {
	st := state.New(10)
	s := &Sim{excessLossDB: 20, excessSet: true}
	Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	// A measurement, planted the way validate.compare would have left it.
	st.Handle("test.residuals", func(w *state.World, p any) (any, error) {
		w.Residuals = p.(*state.Residuals)
		return nil, nil
	})
	if _, err := st.Do(ctx, "test.residuals", &state.Residuals{Matched: 100, MedianDB: 6}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "validate.calibrate", nil); err != nil {
		t.Fatal(err)
	}
	if s.excessLossDB != 26 {
		t.Fatalf("excess loss = %.1f dB; the +6 dB residual was measured on top "+
			"of 20 dB and the total must keep both", s.excessLossDB)
	}

	// An explicit figure stays absolute: an operator stating a total means it.
	if _, err := st.Do(ctx, "validate.calibrate", map[string]any{"db": float64(23)}); err != nil {
		t.Fatal(err)
	}
	if s.excessLossDB != 23 {
		t.Fatalf("explicit db = %.1f, want the stated 23", s.excessLossDB)
	}
}
