package replay_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/provider"
	"github.com/MeshBench/meshbench/internal/replay"
)

func at(sec int) time.Time {
	return time.Date(2026, 8, 9, 12, 0, sec, 0, time.UTC)
}

func opts() replay.Options {
	return replay.Options{SF: 10, BandwidthHz: 250_000, CodingRate: 1, DefaultPayloadBytes: 32}
}

// A flood is heard once per hop. Treating each reception as a separate origin
// transmission produces a mesh far busier than the one recorded — which is the
// opposite of the error a congestion study can afford.
func TestFloodCollapsesToOneTransmission(t *testing.T) {
	rx := []provider.Reception{
		{PacketID: "abc", Origin: "gw", Receiver: "r1", At: at(0)},
		{PacketID: "abc", Origin: "gw", Receiver: "r2", At: at(0)},
		{PacketID: "abc", Origin: "gw", Receiver: "r3", At: at(0)},
		{PacketID: "def", Origin: "gw", Receiver: "r1", At: at(10)},
	}
	s, err := replay.Build(rx, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Transmissions) != 2 {
		t.Fatalf("recovered %d transmissions from 2 distinct packets", len(s.Transmissions))
	}
	if s.DuplicateHops != 2 {
		t.Errorf("counted %d repeat receptions, want 2", s.DuplicateHops)
	}
	if len(s.Transmissions[0].HeardBy) != 3 {
		t.Errorf("the first packet was heard by %v", s.Transmissions[0].HeardBy)
	}
}

// Two observers reporting the same packet a second apart have not observed a
// one-second propagation delay. At LoRa ranges the true spread is microseconds,
// so it is a clock, and a replay that presents it as a measurement is lying.
func TestClockSkewIsCalledWhatItIs(t *testing.T) {
	rx := []provider.Reception{
		{PacketID: "abc", Origin: "gw", Receiver: "r1", At: at(0)},
		{PacketID: "abc", Origin: "gw", Receiver: "r2", At: at(4)},
	}
	s, err := replay.Build(rx, opts())
	if err != nil {
		t.Fatal(err)
	}
	if !s.ClockSkewWarning {
		t.Error("a four-second spread was not flagged")
	}
	if len(s.Transmissions[0].Assumed) == 0 {
		t.Error("the transmission does not record that its time is a bound")
	}
	if !strings.Contains(s.Describe(), "clocks disagree") {
		t.Errorf("the description does not explain the skew:\n%s", s.Describe())
	}
	// The transmit time must be the earliest reception, not an average of two
	// disagreeing clocks.
	if s.Transmissions[0].At != 0 {
		t.Errorf("transmit time is %v; it should be bounded by the earliest reception", s.Transmissions[0].At)
	}
}

// What was never heard was never recorded, so utilisation is a lower bound.
// Presenting it as a measurement would understate congestion, which is the
// wrong direction for the question it answers.
func TestUtilisationIsStatedAsALowerBound(t *testing.T) {
	var rx []provider.Reception
	for i := 0; i < 10; i++ {
		rx = append(rx, provider.Reception{
			PacketID: string(rune('a' + i)), Origin: "gw", Receiver: "r1", At: at(i),
		})
	}
	s, err := replay.Build(rx, opts())
	if err != nil {
		t.Fatal(err)
	}
	u := s.ChannelUtilisation()
	if u <= 0 || u > 1 {
		t.Errorf("utilisation %.3f is not a fraction", u)
	}
	if !strings.Contains(s.Describe(), "LOWER BOUND") {
		t.Errorf("utilisation is not qualified:\n%s", s.Describe())
	}
}

// Airtime has to come from the same formula the firmware uses, or the recovered
// occupancy is about a different radio.
func TestAirtimeMatchesTheFirmware(t *testing.T) {
	rx := []provider.Reception{{PacketID: "a", Origin: "gw", Receiver: "r1", At: at(0)}}
	sf10, err := replay.Build(rx, opts())
	if err != nil {
		t.Fatal(err)
	}
	slow := opts()
	slow.SF = 12
	sf12, err := replay.Build(rx, slow)
	if err != nil {
		t.Fatal(err)
	}
	if sf12.Transmissions[0].AirtimeMs <= sf10.Transmissions[0].AirtimeMs*2 {
		t.Errorf("SF12 airtime %.0f ms against SF10 %.0f ms — the formula is not being applied",
			sf12.Transmissions[0].AirtimeMs, sf10.Transmissions[0].AirtimeMs)
	}
}

// An assumed payload length is an assumption and must say so. A replay built on
// invented numbers that does not name them is worse than one that refuses.
func TestAssumptionsAreRecorded(t *testing.T) {
	noPayload, err := replay.Build(
		[]provider.Reception{{PacketID: "a", Origin: "gw", Receiver: "r1", At: at(0)}}, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(noPayload.Transmissions[0].Assumed) == 0 {
		t.Error("an assumed payload length was not recorded as an assumption")
	}

	withPayload, err := replay.Build([]provider.Reception{
		{PacketID: "a", Origin: "gw", Receiver: "r1", At: at(0), RawPayload: make([]byte, 64)},
	}, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(withPayload.Transmissions[0].Assumed) != 0 {
		t.Errorf("a real payload was still flagged as assumed: %v", withPayload.Transmissions[0].Assumed)
	}
	if withPayload.Transmissions[0].PayloadBytes != 64 {
		t.Errorf("payload recorded as %d bytes, want 64", withPayload.Transmissions[0].PayloadBytes)
	}
}

func TestUnrecoverableReceptionsAreCounted(t *testing.T) {
	s, err := replay.Build([]provider.Reception{
		{PacketID: "", Origin: "gw", Receiver: "r1", At: at(0)},  // no identity
		{PacketID: "b", Origin: "gw", Receiver: "r1"},            // no time
		{PacketID: "c", Origin: "", Receiver: "r1", At: at(1)},   // no origin
		{PacketID: "d", Origin: "gw", Receiver: "r1", At: at(2)}, // fine
	}, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Transmissions) != 1 {
		t.Errorf("recovered %d transmissions, want 1", len(s.Transmissions))
	}
	if s.DroppedNoTime != 1 || s.DroppedNoOrigin != 2 {
		t.Errorf("dropped accounting: noTime=%d noOrigin=%d", s.DroppedNoTime, s.DroppedNoOrigin)
	}
}

func TestEmptyRecordingSaysSo(t *testing.T) {
	s, err := replay.Build(nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Describe(), "Nothing recoverable") {
		t.Errorf("an empty session should say so plainly:\n%s", s.Describe())
	}
}

func TestRejectsBadSpreadingFactor(t *testing.T) {
	bad := opts()
	bad.SF = 3
	if _, err := replay.Build(nil, bad); err == nil {
		t.Fatal("SF3 was accepted")
	}
}
